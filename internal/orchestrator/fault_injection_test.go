package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

type createRaceTaskRepository struct {
	TaskRepository
	arrived chan struct{}
	release chan struct{}
	gets    atomic.Int32
	mu      sync.Mutex
	task    *model.Task
}

func (r *createRaceTaskRepository) GetByDedupeKey(_ context.Context, dedupeKey string) (*model.Task, error) {
	r.mu.Lock()
	var snapshot *model.Task
	if r.task != nil && r.task.DedupeKey == dedupeKey {
		copy := *r.task
		snapshot = &copy
	}
	r.mu.Unlock()
	if r.gets.Add(1) <= 2 {
		r.arrived <- struct{}{}
		<-r.release
	}
	if snapshot == nil {
		return nil, repository.ErrNotFound
	}
	return snapshot, nil
}

func (r *createRaceTaskRepository) Create(_ context.Context, task *model.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task != nil {
		return errors.New("unique task dedupe key")
	}
	copy := *task
	r.task = &copy
	return nil
}

func TestCreateTaskConcurrentSameDedupeReturnsOneTask(t *testing.T) {
	repo := &createRaceTaskRepository{
		arrived: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	orch := New(repo, nil)
	ctx := context.Background()
	tasks := []*model.Task{
		{TaskID: "task-race-1", DedupeKey: "same-business-event", Type: "review_mr", Role: "ReviewerWorker"},
		{TaskID: "task-race-2", DedupeKey: "same-business-event", Type: "review_mr", Role: "ReviewerWorker"},
	}

	results := make(chan *model.Task, 2)
	errs := make(chan error, 2)
	for _, task := range tasks {
		task := task
		go func() {
			created, err := orch.CreateTask(ctx, task)
			results <- created
			errs <- err
		}()
	}
	<-repo.arrived
	<-repo.arrived
	close(repo.release)

	first, second := <-results, <-results
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent CreateTask returned error: %v", err)
		}
	}
	if first == nil || second == nil || first.TaskID != second.TaskID {
		t.Fatalf("expected both callers to receive one task, got %#v and %#v", first, second)
	}
}

type dispatchBarrierTaskRepository struct {
	TaskRepository
	arrived chan struct{}
	release chan struct{}
	lists   atomic.Int32
}

func (r *dispatchBarrierTaskRepository) ListDispatchable(ctx context.Context, limit int) ([]model.Task, error) {
	tasks, err := r.TaskRepository.ListDispatchable(ctx, limit)
	if err != nil {
		return nil, err
	}
	if r.lists.Add(1) <= 2 {
		r.arrived <- struct{}{}
		<-r.release
	}
	return tasks, nil
}

func TestConcurrentTicksMoveUnsupportedTaskToHumanRequiredOnce(t *testing.T) {
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	baseTasks := repository.NewTaskRepository(db)
	tasks := &dispatchBarrierTaskRepository{
		TaskRepository: baseTasks,
		arrived:        make(chan struct{}, 2),
		release:        make(chan struct{}),
	}
	runs := repository.NewRunRepository(db)
	orch := New(
		tasks,
		runs,
		WithClock(func() time.Time { return time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC) }),
	)
	repositoryID := fixtureRepositoryID
	task := &model.Task{TaskID: "task-tick-race", DedupeKey: "tick-race", Type: "verification", Role: "TesterWorker", RepositoryID: &repositoryID, Status: model.TaskStatusPending}
	if err := baseTasks.Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	results := make(chan *TickResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, err := orch.Tick(context.Background(), TickRequest{})
		results <- result
		errs <- err
	}()
	<-tasks.arrived

	// Tick 内部包含非幂等副作用（developer 交付 git 操作），并发方必须被
	// single-flight 互斥拒绝，而不是参与竞争。
	if _, err := orch.Tick(context.Background(), TickRequest{}); !errors.Is(err, ErrTickInProgress) {
		t.Fatalf("concurrent tick must be rejected with ErrTickInProgress, got %v", err)
	}
	close(tasks.release)

	if err := <-errs; err != nil {
		t.Fatalf("first tick returned error: %v", err)
	}
	result := <-results
	if len(result.DispatchFailures) != 1 {
		t.Fatalf("expected exactly one human-required transition, got %#v", result.DispatchFailures)
	}
}
