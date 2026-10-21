package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
)

type startRunTakeoverTasks struct {
	TaskRepository
	afterStart func()
}

func (r *startRunTakeoverTasks) StartRunAtomic(ctx context.Context, taskID string, run *model.Run, leaseUntil, startedAt time.Time) (*model.Task, error) {
	task, err := r.TaskRepository.StartRunAtomic(ctx, taskID, run, leaseUntil, startedAt)
	if err == nil && r.afterStart != nil {
		r.afterStart()
	}
	return task, err
}

func TestDeveloperDispatchRejectsRunStartAfterTakeover(t *testing.T) {
	for _, backend := range []string{"sqlite", "mysql"} {
		t.Run(backend, func(t *testing.T) {
			for _, point := range []string{"replacement_running", "replacement_claimed"} {
				t.Run(point, func(t *testing.T) {
					dsn := testDatabaseDSN(t)
					if backend == "mysql" {
						dsn = storetest.NewDatabase(t, storetest.RequireMySQL(t), "start_run_takeover")
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
					ctx := context.Background()
					tasks := repository.NewTaskRepository(first)
					runs := repository.NewRunRepository(first)
					wrappedTasks := &startRunTakeoverTasks{TaskRepository: tasks}
					runner := &fakeDeveloperRunnerCoordinator{}
					now := time.Now().UTC()
					old := New(wrappedTasks, runs,
						WithClock(func() time.Time { return now.Add(-time.Hour) }),
						WithDeveloperRunner(runner, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}),
					)
					replacement := New(repository.NewTaskRepository(second), repository.NewRunRepository(second),
						WithClock(func() time.Time { return now }),
					)
					task := &model.Task{
						TaskID: "task-takeover", DedupeKey: "takeover", Type: "develop_from_issue", Role: "DeveloperWorker",
						Status: model.TaskStatusPending, TargetRepo: "group/project", DeliveryBranch: "agent/task-takeover",
						ContextJSON: `{"project":{"path":"group/project"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"main","access":"writable"}]}`,
					}
					if err := tasks.Create(ctx, task); err != nil {
						t.Fatal(err)
					}
					var takenOver *model.Task
					takeover := func() {
						if _, err := replacement.RecoverTask(ctx, task.TaskID); err != nil {
							t.Fatalf("recover old attempt: %v", err)
						}
						if point == "replacement_claimed" {
							takenOver, err = replacement.ClaimTask(ctx, task.TaskID, "run-replacement")
						} else {
							_, takenOver, err = replacement.StartRun(ctx, task.TaskID, &model.Run{
								RunID: "run-replacement", TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusPending,
							})
						}
						if err != nil {
							t.Fatalf("start replacement attempt: %v", err)
						}
					}
					wrappedTasks.afterStart = takeover
					result, dispatchErr := old.DispatchDeveloperTask(ctx, task.TaskID)
					if takenOver == nil {
						t.Fatal("replacement did not take over")
					}
					current, err := tasks.GetByTaskID(ctx, task.TaskID)
					if err != nil {
						t.Fatal(err)
					}
					if current.CurrentRunID == nil || *current.CurrentRunID != "run-replacement" ||
						current.Status != takenOver.Status || current.AttemptCount != takenOver.AttemptCount ||
						current.LeaseUntil == nil || !current.LeaseUntil.Equal(*takenOver.LeaseUntil) {
						t.Errorf("old start overwrote replacement state: current=%+v replacement=%+v", current, takenOver)
					}
					if !errors.Is(dispatchErr, repository.ErrTaskNotClaimable) || result != nil {
						t.Errorf("old dispatch must lose ownership: result=%+v err=%v", result, dispatchErr)
					}
					if runner.startCalls != 0 {
						t.Errorf("old dispatch started runner %d times after takeover", runner.startCalls)
					}
				})
			}
		})
	}
}
