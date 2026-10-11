package repository

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestTaskRepositoryCreateAndLookup(t *testing.T) {
	db, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewTaskRepository(db)
	ctx := context.Background()

	task := &model.Task{
		DedupeKey:          "gitlab:test-automates:issue:42",
		Type:               "develop_from_issue",
		Role:               "DeveloperWorker",
		SourceRepo:         "test-automates",
		TargetRepo:         "framework",
		Objective:          "fix upstream price validation bug",
		AcceptanceCriteria: "root cause identified and MR created",
		ContextJSON:        `{"issue":"42"}`,
		CallbackURL:        "http://localhost:8080/api/runs/callback",
	}

	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.TaskID == "" {
		t.Fatal("expected task id to be generated")
	}
	if task.Status != model.TaskStatusPending {
		t.Fatalf("expected pending status, got %s", task.Status)
	}

	loaded, err := repo.GetByDedupeKey(ctx, task.DedupeKey)
	if err != nil {
		t.Fatalf("get by dedupe key: %v", err)
	}
	if loaded.TaskID != task.TaskID {
		t.Fatalf("expected task id %s, got %s", task.TaskID, loaded.TaskID)
	}
	if loaded.TargetRepo != "framework" {
		t.Fatalf("expected target repo framework, got %s", loaded.TargetRepo)
	}
}

func TestTaskRepositoryPersistsNullableRepositoryID(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewTaskRepository(db)
	orphan := &model.Task{DedupeKey: "orphan", Type: "develop_from_issue", Role: "DeveloperWorker"}
	if err := repo.Create(context.Background(), orphan); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.GetByTaskID(context.Background(), orphan.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RepositoryID != nil {
		t.Fatalf("expected nil, got %v", loaded.RepositoryID)
	}
	id := "repo_a"
	owned := &model.Task{DedupeKey: "owned", Type: "develop_from_issue", Role: "DeveloperWorker", RepositoryID: &id}
	if err := repo.Create(context.Background(), owned); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByTaskID(context.Background(), owned.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RepositoryID == nil || *got.RepositoryID != "repo_a" {
		t.Fatalf("got %#v", got.RepositoryID)
	}
}

func TestTaskRepositoryResumeHumanRequiredIsAtomic(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewTaskRepository(db)
	ctx := context.Background()
	currentRunID := "run-original"
	task := &model.Task{
		TaskID:       "task-resume-atomic",
		DedupeKey:    "gitlab:issue-develop:15594:2",
		Type:         "develop_from_issue",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusHumanRequired,
		CurrentRunID: &currentRunID,
		ContextJSON:  `{"issue":{"iid":2}}`,
		LastError:    "need API contract",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- repo.ResumeHumanRequired(ctx, task.DedupeKey, `{"issue":{"iid":2},"resume":{"note_id":169424}}`)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	succeeded := 0
	notClaimable := 0
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrTaskNotClaimable):
			notClaimable++
		default:
			t.Fatalf("unexpected resume error: %v", err)
		}
	}
	if succeeded != 1 || notClaimable != 1 {
		t.Fatalf("expected one successful and one rejected resume, got %d/%d", succeeded, notClaimable)
	}

	stored, err := repo.GetByDedupeKey(ctx, task.DedupeKey)
	if err != nil {
		t.Fatalf("load resumed task: %v", err)
	}
	if stored.Status != model.TaskStatusPending || stored.CurrentRunID != nil || stored.LastError != "" {
		t.Fatalf("unexpected resumed task: %#v", stored)
	}
}

func TestTaskRepositoryResumeReworkIsAtomic(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewTaskRepository(db)
	ctx := context.Background()
	currentRunID := "run-rework-original"
	lease := time.Now().Add(time.Minute)
	task := &model.Task{
		TaskID:       "task-rework-resume",
		DedupeKey:    "gitlab:mr-rework:1001:21",
		Type:         "rework_merge_request",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatus("waiting_merge"),
		CurrentRunID: &currentRunID,
		LeaseUntil:   &lease,
		ContextJSON:  `{"merge_request":{"iid":21}}`,
		LastError:    "old failure",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- repo.ResumeRework(ctx, task.DedupeKey, `{"merge_request":{"iid":21},"rework_request":{"note_id":20002}}`)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	succeeded, notClaimable := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrTaskNotClaimable):
			notClaimable++
		default:
			t.Fatalf("unexpected resume error: %v", err)
		}
	}
	if succeeded != 1 || notClaimable != 1 {
		t.Fatalf("expected one successful and one rejected resume, got %d/%d", succeeded, notClaimable)
	}

	stored, err := repo.GetByDedupeKey(ctx, task.DedupeKey)
	if err != nil {
		t.Fatalf("load resumed task: %v", err)
	}
	if stored.Status != model.TaskStatusPending || stored.CurrentRunID != nil || stored.LeaseUntil != nil || stored.LastError != "" {
		t.Fatalf("unexpected resumed task: %#v", stored)
	}
	if !strings.Contains(stored.ContextJSON, `"note_id":20002`) {
		t.Fatalf("expected refreshed context, got %s", stored.ContextJSON)
	}
}

func TestTaskRepositoryResumeReworkRejectsNormalOrActiveTask(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewTaskRepository(db)
	ctx := context.Background()

	for _, task := range []*model.Task{
		{TaskID: "task-normal", DedupeKey: "normal", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusHumanRequired},
		{TaskID: "task-active", DedupeKey: "active", Type: "rework_merge_request", Role: "DeveloperWorker", Status: model.TaskStatusRunning},
	} {
		if err := repo.Create(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.TaskID, err)
		}
		if err := repo.ResumeRework(ctx, task.DedupeKey, `{}`); !errors.Is(err, ErrTaskNotClaimable) {
			t.Fatalf("expected %s to be not claimable, got %v", task.TaskID, err)
		}
	}
}

func TestTaskRepositoryListExpiredLeases(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewTaskRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	olderExpired := now.Add(-10 * time.Minute)
	newerExpired := now.Add(-5 * time.Minute)
	activeLease := now.Add(10 * time.Minute)

	tasks := []*model.Task{
		{
			TaskID:     "task-expired-running-newer",
			DedupeKey:  "dedupe-expired-running-newer",
			Type:       "bugfix",
			Role:       "DeveloperWorker",
			Status:     model.TaskStatusRunning,
			LeaseUntil: &newerExpired,
		},
		{
			TaskID:     "task-expired-claimed-older",
			DedupeKey:  "dedupe-expired-claimed-older",
			Type:       "bugfix",
			Role:       "TesterWorker",
			Status:     model.TaskStatusClaimed,
			LeaseUntil: &olderExpired,
		},
		{
			TaskID:     "task-active-running",
			DedupeKey:  "dedupe-active-running",
			Type:       "bugfix",
			Role:       "DeveloperWorker",
			Status:     model.TaskStatusRunning,
			LeaseUntil: &activeLease,
		},
		{
			TaskID:     "task-expired-pending",
			DedupeKey:  "dedupe-expired-pending",
			Type:       "bugfix",
			Role:       "DeveloperWorker",
			Status:     model.TaskStatusPending,
			LeaseUntil: &olderExpired,
		},
		{
			TaskID:     "task-expired-succeeded",
			DedupeKey:  "dedupe-expired-succeeded",
			Type:       "bugfix",
			Role:       "DeveloperWorker",
			Status:     model.TaskStatusSucceeded,
			LeaseUntil: &olderExpired,
		},
	}
	for _, task := range tasks {
		if err := repo.Create(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.TaskID, err)
		}
	}

	expired, err := repo.ListExpiredLeases(ctx, now, 10)
	if err != nil {
		t.Fatalf("list expired leases: %v", err)
	}

	if len(expired) != 2 {
		t.Fatalf("expected two expired recoverable tasks, got %d", len(expired))
	}
	if expired[0].TaskID != "task-expired-claimed-older" {
		t.Fatalf("expected older claimed task first, got %s", expired[0].TaskID)
	}
	if expired[1].TaskID != "task-expired-running-newer" {
		t.Fatalf("expected newer running task second, got %s", expired[1].TaskID)
	}
}

func TestTaskRepositoryListDispatchable(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewTaskRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	oldest := now.Add(-3 * time.Minute)
	middle := now.Add(-2 * time.Minute)
	newest := now.Add(-time.Minute)

	tasks := []*model.Task{
		{
			TaskID:    "task-dispatch-running",
			DedupeKey: "dedupe-dispatch-running",
			Type:      "bugfix",
			Role:      "DeveloperWorker",
			Status:    model.TaskStatusRunning,
			UpdatedAt: oldest,
		},
		{
			TaskID:    "task-dispatch-retry",
			DedupeKey: "dedupe-dispatch-retry",
			Type:      "bugfix",
			Role:      "DeveloperWorker",
			Status:    model.TaskStatusRetryPending,
			UpdatedAt: middle,
		},
		{
			TaskID:    "task-dispatch-pending",
			DedupeKey: "dedupe-dispatch-pending",
			Type:      "bugfix",
			Role:      "TesterWorker",
			Status:    model.TaskStatusPending,
			UpdatedAt: newest,
		},
		{
			TaskID:    "task-dispatch-waiting",
			DedupeKey: "dedupe-dispatch-waiting",
			Type:      "bugfix",
			Role:      "TesterWorker",
			Status:    model.TaskStatusWaitingUpstream,
			UpdatedAt: oldest,
		},
	}
	for _, task := range tasks {
		if err := repo.Create(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.TaskID, err)
		}
	}

	dispatchable, err := repo.ListDispatchable(ctx, 10)
	if err != nil {
		t.Fatalf("list dispatchable: %v", err)
	}

	if len(dispatchable) != 2 {
		t.Fatalf("expected two dispatchable tasks, got %d", len(dispatchable))
	}
	if dispatchable[0].TaskID != "task-dispatch-retry" {
		t.Fatalf("expected retry task first, got %s", dispatchable[0].TaskID)
	}
	if dispatchable[1].TaskID != "task-dispatch-pending" {
		t.Fatalf("expected pending task second, got %s", dispatchable[1].TaskID)
	}

	limited, err := repo.ListDispatchable(ctx, 1)
	if err != nil {
		t.Fatalf("list limited dispatchable: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected one limited dispatchable task, got %d", len(limited))
	}
	if limited[0].TaskID != "task-dispatch-retry" {
		t.Fatalf("expected limited result to keep ordering, got %s", limited[0].TaskID)
	}
}

func TestTaskRepositoryListDispatchableAfterClaimedCursor(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatal(err)
	}
	repo := NewTaskRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	keys := []string{"later-timestamp", "cursor", "same-timestamp", "running"}
	for i, updatedAt := range []time.Time{now.Add(time.Minute), now, now, now.Add(2 * time.Minute)} {
		task := &model.Task{TaskID: keys[i], DedupeKey: keys[i], Role: "DeveloperWorker", Type: "bugfix", Status: model.TaskStatusPending, UpdatedAt: updatedAt}
		if i == 3 {
			task.Status = model.TaskStatusRunning
		}
		if err := repo.Create(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	first, err := repo.ListDispatchable(ctx, 1)
	if err != nil || len(first) != 1 || first[0].TaskID != "cursor" {
		t.Fatalf("unexpected first page: tasks=%#v err=%v", first, err)
	}
	if err := repo.Claim(ctx, first[0].TaskID, "cursor-run", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	throughID, err := repo.MaxTaskID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.ListDispatchableAfter(ctx, &first[0], 1, throughID)
	if err != nil || len(second) != 1 || second[0].TaskID != "same-timestamp" {
		t.Fatalf("claimed cursor must not skip equal-timestamp successor: tasks=%#v err=%v", second, err)
	}
	last, err := repo.ListDispatchableAfter(ctx, &second[0], 10, throughID)
	if err != nil || len(last) != 1 || last[0].TaskID != "later-timestamp" {
		t.Fatalf("later timestamp must take priority over ID, excluding running tasks: tasks=%#v err=%v", last, err)
	}
}

func TestTaskRepositoryBlockRunAndRequireHumanRollsBackOnTaskFailure(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Force task updates to fail so the transaction must roll back the run update.
	if err := db.Exec(`CREATE TRIGGER tasks_last_error_limit BEFORE UPDATE ON tasks WHEN length(NEW.last_error) > 5 BEGIN SELECT RAISE(ABORT, 'last_error too long'); END`).Error; err != nil {
		t.Fatalf("install trigger: %v", err)
	}

	repo := NewTaskRepository(db)
	runs := NewRunRepository(db)
	ctx := context.Background()
	runID := "run-block-atomic"
	task := &model.Task{
		TaskID: "task-block-atomic", DedupeKey: "block-atomic", Type: "develop_from_issue", Role: "DeveloperWorker",
		Status: model.TaskStatusRunning, CurrentRunID: &runID, ContextJSON: `{}`,
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	if err := repo.BlockRunAndRequireHuman(ctx, runID, "this message exceeds the constraint", time.Now()); err == nil {
		t.Fatal("expected task constraint failure to abort the transaction")
	}

	storedRun, err := runs.GetByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if storedRun.Status != model.RunStatusRunning {
		t.Fatalf("run update was not rolled back: %#v", storedRun)
	}
	storedTask, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if storedTask.Status != model.TaskStatusRunning || storedTask.LastError != "" {
		t.Fatalf("task update was not rolled back: %#v", storedTask)
	}
}

func TestTaskRepositoryFindsActiveReviewerByDedupePrefix(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewTaskRepository(db)
	ctx := context.Background()

	mkTask := func(taskID, dedupeKey string, role string, status model.TaskStatus) *model.Task {
		return &model.Task{TaskID: taskID, DedupeKey: dedupeKey, Type: "review_mr", Role: role, Status: status}
	}
	if err := repo.Create(ctx, mkTask("task-mr-21-full", "gitlab:mr-review:group/project:21:aaaa", "ReviewerWorker", model.TaskStatusRunning)); err != nil {
		t.Fatalf("create full review: %v", err)
	}
	if err := repo.Create(ctx, mkTask("task-mr-21-inc", "gitlab:mr-review:group/project:21:bbbb", "ReviewerWorker", model.TaskStatusPending)); err != nil {
		t.Fatalf("create incremental review: %v", err)
	}
	if err := repo.Create(ctx, mkTask("task-mr-210-similar", "gitlab:mr-review:group/project:210:cccc", "ReviewerWorker", model.TaskStatusPending)); err != nil {
		t.Fatalf("create similar iid review: %v", err)
	}
	if err := repo.Create(ctx, mkTask("task-mr-21-done", "gitlab:mr-review:group/project:21:dddd", "ReviewerWorker", model.TaskStatusSucceeded)); err != nil {
		t.Fatalf("create settled review: %v", err)
	}
	if err := repo.Create(ctx, mkTask("task-issue-dev", "gitlab:issue-develop:group/project:21", "DeveloperWorker", model.TaskStatusPending)); err != nil {
		t.Fatalf("create developer task: %v", err)
	}

	prefix := "gitlab:mr-review:group/project:21:"
	active, err := repo.HasActiveReviewerTaskForDedupePrefix(ctx, prefix, "task-mr-21-inc")
	if err != nil || !active {
		t.Fatalf("running review on the same MR must defer the newcomer: active=%v err=%v", active, err)
	}
	settled, err := repo.HasActiveReviewerTaskForDedupePrefix(ctx, "gitlab:mr-review:group/project:22:", "task-mr-21-inc")
	if err != nil || settled {
		t.Fatalf("a different iid prefix must not match: active=%v err=%v", settled, err)
	}
	earlier, err := repo.GetByTaskID(ctx, "task-mr-21-full")
	if err != nil {
		t.Fatalf("get earlier review: %v", err)
	}
	earlier.Status = model.TaskStatusSucceeded
	if err := repo.Update(ctx, earlier); err != nil {
		t.Fatalf("settle earlier review: %v", err)
	}
	active, err = repo.HasActiveReviewerTaskForDedupePrefix(ctx, prefix, "task-mr-21-inc")
	if err != nil || active {
		t.Fatalf("settled predecessor must stop deferring: active=%v err=%v", active, err)
	}
	active, err = repo.HasActiveReviewerTaskForDedupePrefix(ctx, "gitlab:mr-review:group/project:210:", "task-mr-210-similar")
	if err != nil || active {
		t.Fatalf("own prefix with only self pending must not self-defer: active=%v err=%v", active, err)
	}
}

func TestReviewerDedupePrefixWaitsForPredecessorsAndExecutingSuccessors(t *testing.T) {
	for _, status := range []model.TaskStatus{
		model.TaskStatusPending, model.TaskStatusRetryPending, model.TaskStatusClaimed,
		model.TaskStatusRunning, model.TaskStatusFinalizing, model.TaskStatusSucceeded,
		model.TaskStatusBlocked, model.TaskStatusHumanRequired, model.TaskStatusWaitingMerge,
	} {
		t.Run(string(status), func(t *testing.T) {
			db, err := store.Open("file:" + model.NewID("review_order") + "?mode=memory&cache=shared")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Migrate(db, &model.Task{}); err != nil {
				t.Fatal(err)
			}
			repo := NewTaskRepository(db)
			ctx := context.Background()
			prefix := "github:mr-review:42:21:"
			a := &model.Task{TaskID: "head-a", DedupeKey: prefix + "a", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusRetryPending}
			b := &model.Task{TaskID: "head-b", DedupeKey: prefix + "b", Type: "review_mr", Role: "ReviewerWorker", Status: status}
			for _, task := range []*model.Task{a, b} {
				if err := repo.Create(ctx, task); err != nil {
					t.Fatal(err)
				}
			}
			active, err := repo.HasActiveReviewerTaskForDedupePrefix(ctx, prefix, a.TaskID)
			want := status == model.TaskStatusClaimed || status == model.TaskStatusRunning || status == model.TaskStatusFinalizing
			if err != nil || active != want {
				t.Fatalf("successor %s: active=%v want=%v err=%v", status, active, want, err)
			}
			active, err = repo.HasActiveReviewerTaskForDedupePrefix(ctx, prefix, b.TaskID)
			if err != nil || !active {
				t.Fatalf("retry-pending predecessor must block: active=%v err=%v", active, err)
			}
		})
	}
}
