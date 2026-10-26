package repository

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

// TestMySQLStopCrossProcessSingleDecision 两个真实进程在同一隔离 MySQL 库上
// 同时停止同一 Task：只允许一个 StopRequest 持久决定，两个调用返回同一 StopID。
func TestMySQLStopCrossProcessSingleDecision(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "stop_procrace")
	setup, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("setup open: %v", err)
	}
	if err := store.Migrate(setup, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{}, &model.StopRequest{}, &model.ActionAdmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	runID := "run-process"
	task := &model.Task{TaskID: "task-process", DedupeKey: "task-process", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, Status: model.RunStatusRunning}
	execution := &model.AgentExecution{RunID: runID, ExecutionID: "execution-process", Provider: "codex", Status: model.AgentExecutionStatusRunning, RequestHash: "identity", RunnerEndpoint: "http://runner:8090"}
	for _, row := range []any{task, run, execution} {
		if err := setup.Create(row).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := setup.Exec("CREATE TABLE stop_ready(holder varchar(16) primary key)").Error; err != nil {
		t.Fatalf("ready table: %v", err)
	}
	if err := setup.Exec("CREATE TABLE stop_result(holder varchar(16) primary key, stop_id varchar(128) not null, err varchar(255) not null)").Error; err != nil {
		t.Fatalf("result table: %v", err)
	}
	if sqlDB, err := setup.DB(); err == nil {
		_ = sqlDB.Close()
	}

	type childResult struct {
		holder string
		output string
		err    error
	}
	results := make(chan childResult, 2)
	var wg sync.WaitGroup
	for _, holder := range []string{"A", "B"} {
		wg.Add(1)
		go func(holder string) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestMySQLStopCrossProcessChild$", "-test.v")
			cmd.Env = append(os.Environ(),
				"SDLC_TEST_STOP_CHILD_DSN="+dsn,
				"SDLC_TEST_STOP_CHILD_HOLDER="+holder,
			)
			out, err := cmd.CombinedOutput()
			results <- childResult{holder: holder, output: string(out), err: err}
		}(holder)
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			t.Fatalf("child %s failed: %v\n%s", result.holder, result.err, result.output)
		}
	}

	verify, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("verify open: %v", err)
	}
	defer func() {
		if sqlDB, err := verify.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	var count int64
	if err := verify.Model(&model.StopRequest{}).Where("task_id = ?", task.TaskID).Count(&count).Error; err != nil {
		t.Fatalf("count stops: %v", err)
	}
	if count != 1 {
		t.Fatalf("stop decisions = %d, want exactly 1", count)
	}
	var recorded model.StopRequest
	if err := verify.Where("task_id = ?", task.TaskID).First(&recorded).Error; err != nil {
		t.Fatalf("read stop: %v", err)
	}
	var duplicates int64
	if err := verify.Raw("SELECT COUNT(DISTINCT stop_id) FROM stop_result").Scan(&duplicates).Error; err != nil {
		t.Fatalf("count distinct child stop ids: %v", err)
	}
	if duplicates != 1 {
		t.Fatalf("child stop ids = %d, want the same persisted decision", duplicates)
	}
	var ids []string
	if err := verify.Raw("SELECT stop_id FROM stop_result").Scan(&ids).Error; err != nil {
		t.Fatalf("read child stop ids: %v", err)
	}
	for _, id := range ids {
		if id != recorded.StopID {
			t.Fatalf("child returned stop %q, persisted %q", id, recorded.StopID)
		}
	}
	var stored model.Task
	if err := verify.Where("task_id = ?", task.TaskID).First(&stored).Error; err != nil || stored.Status != model.TaskStatusInterrupted {
		t.Fatalf("task status = %s err=%v", stored.Status, err)
	}
	var storedRun model.Run
	if err := verify.Where("run_id = ?", runID).First(&storedRun).Error; err != nil || storedRun.Status != model.RunStatusInterrupted {
		t.Fatalf("run status = %s err=%v", storedRun.Status, err)
	}
	var errorsText []string
	if err := verify.Raw("SELECT err FROM stop_result WHERE err <> ''").Scan(&errorsText).Error; err != nil {
		t.Fatalf("read child errors: %v", err)
	}
	if len(errorsText) != 0 {
		t.Fatalf("child stop errors: %v", errorsText)
	}
}

// TestMySQLStopCrossProcessChild 子进程入口：经汇合表对齐后停止固定 Task，
// 记录返回的 StopID 或错误供父测试断言单一持久决定。
func TestMySQLStopCrossProcessChild(t *testing.T) {
	dsn := os.Getenv("SDLC_TEST_STOP_CHILD_DSN")
	holder := os.Getenv("SDLC_TEST_STOP_CHILD_HOLDER")
	if dsn == "" || holder == "" {
		t.Skip("child helper for TestMySQLStopCrossProcessSingleDecision")
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("child open: %v", err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := db.Exec("INSERT INTO stop_ready(holder) VALUES (?)", holder).Error; err != nil {
		t.Fatalf("rendezvous insert: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		var ready int64
		if err := db.Raw("SELECT COUNT(*) FROM stop_ready").Scan(&ready).Error; err != nil {
			t.Fatalf("rendezvous poll: %v", err)
		}
		if ready >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child %s rendezvous timed out", holder)
		}
		time.Sleep(25 * time.Millisecond)
	}
	decision, stopErr := NewStopRepository(db).Stop(context.Background(), StopTarget{TaskID: "task-process"}, "admin-"+holder, "", 3)
	stopID, errText := "", ""
	if stopErr != nil {
		errText = stopErr.Error()
	} else if decision == nil || decision.Stop == nil {
		errText = "missing stop decision"
	} else {
		stopID = decision.Stop.StopID
	}
	if err := db.Exec("INSERT INTO stop_result(holder, stop_id, err) VALUES (?, ?, ?)", holder, stopID, errText).Error; err != nil {
		t.Fatalf("record result: %v", err)
	}
	if stopErr != nil {
		t.Fatalf("child stop: %v", stopErr)
	}
}
