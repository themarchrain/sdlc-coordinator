package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
)

func TestStartRunAtomicConcurrentClaims(t *testing.T) {
	for _, backend := range []string{"sqlite", "mysql"} {
		t.Run(backend, func(t *testing.T) {
			dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
			if backend == "mysql" {
				dsn = storetest.NewDatabase(t, storetest.RequireMySQL(t), "atomic_start")
			}
			first, err := store.Open(dsn)
			if err != nil {
				t.Fatal(err)
			}
			second, err := store.Open(dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				for _, db := range []*gorm.DB{first, second} {
					if sqlDB, err := db.DB(); err == nil {
						_ = sqlDB.Close()
					}
				}
			})
			if err := store.Migrate(first, &model.Task{}, &model.Run{}); err != nil {
				t.Fatal(err)
			}
			task := &model.Task{TaskID: "atomic", DedupeKey: "atomic", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusRetryPending, AttemptCount: 2}
			if err := first.Create(task).Error; err != nil {
				t.Fatal(err)
			}
			ready := make(chan struct{}, 2)
			release := make(chan struct{})
			type startResult struct {
				task *model.Task
				err  error
			}
			results := make(chan startResult, 2)
			var wg sync.WaitGroup
			for i, db := range []*gorm.DB{first, second} {
				wg.Add(1)
				go func(i int, db *gorm.DB) {
					defer wg.Done()
					ready <- struct{}{}
					<-release
					now := time.Now().UTC()
					run := &model.Run{RunID: fmt.Sprintf("run-%d", i), WorkerRole: task.Role}
					current, err := NewTaskRepository(db).StartRunAtomic(context.Background(), task.TaskID, run, now.Add(time.Hour), now)
					results <- startResult{current, err}
				}(i, db)
			}
			<-ready
			<-ready
			close(release)
			wg.Wait()
			close(results)
			wins := 0
			for result := range results {
				if result.err == nil {
					wins++
					if result.task == nil || result.task.Status != model.TaskStatusRunning || result.task.AttemptCount != 3 || result.task.CurrentRunID == nil || result.task.LeaseUntil == nil {
						t.Errorf("start returned stale snapshot: %+v", result.task)
					}
				} else if !errors.Is(result.err, ErrTaskNotClaimable) {
					t.Fatalf("unexpected start error: %v", result.err)
				}
			}
			if wins != 1 {
				t.Fatalf("successful starts = %d, want 1", wins)
			}
			current, err := NewTaskRepository(first).GetByTaskID(context.Background(), task.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			var runs []model.Run
			if err := first.Where("task_id = ?", task.TaskID).Find(&runs).Error; err != nil {
				t.Fatal(err)
			}
			if len(runs) != 1 || current.Status != model.TaskStatusRunning || current.AttemptCount != 3 || current.CurrentRunID == nil || *current.CurrentRunID != runs[0].RunID || runs[0].Status != model.RunStatusRunning || runs[0].StartedAt == nil || current.LeaseUntil == nil {
				t.Fatalf("non-atomic start: task=%+v runs=%+v", current, runs)
			}
		})
	}
}
