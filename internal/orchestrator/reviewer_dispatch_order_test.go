package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

func TestDispatchReviewerTaskDefersToActivePredecessorOnSameMR(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	orch := New(tasks, runs, WithReviewerRunner(&fakeReviewerRunnerCoordinator{}, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))

	earlier := reviewerRunnerTickTask("task-mr-21-earlier", model.TaskStatusRunning)
	earlier.DedupeKey = "gitlab:mr-review:24207:21:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := tasks.Create(ctx, earlier); err != nil {
		t.Fatalf("create earlier review: %v", err)
	}
	later := reviewerRunnerTickTask("task-mr-21-later", model.TaskStatusPending)
	later.DedupeKey = "gitlab:mr-review:24207:21:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := tasks.Create(ctx, later); err != nil {
		t.Fatalf("create later review: %v", err)
	}
	otherMR := reviewerRunnerTickTask("task-mr-22-other", model.TaskStatusPending)
	otherMR.DedupeKey = "gitlab:mr-review:24207:22:cccccccccccccccccccccccccccccccc"
	otherMR.ContextJSON = strings.Replace(otherMR.ContextJSON, `"iid":21`, `"iid":22`, 1)
	if err := tasks.Create(ctx, otherMR); err != nil {
		t.Fatalf("create other-MR review: %v", err)
	}

	if _, err := orch.DispatchReviewerTask(ctx, "task-mr-21-later"); !errors.Is(err, ErrReviewerDispatchDeferred) {
		t.Fatalf("same-MR dispatch must be deferred while an earlier review is active: %v", err)
	}
	stored, err := tasks.GetByTaskID(ctx, "task-mr-21-later")
	if err != nil || stored.Status != model.TaskStatusPending || stored.CurrentRunID != nil {
		t.Fatalf("deferred dispatch must not claim the task or create a run: task=%#v err=%v", stored, err)
	}

	if _, err := orch.DispatchReviewerTask(ctx, "task-mr-22-other"); err != nil {
		t.Fatalf("a different MR must not be deferred: %v", err)
	}
	if result, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10}); err != nil {
		t.Fatalf("tick: %v", err)
	} else if len(result.DispatchFailures) != 0 {
		t.Fatalf("deferred dispatch must not be recorded as a tick failure: %#v", result.DispatchFailures)
	}
}

func TestReviewerDedupePrefixUsesPersistedIdentity(t *testing.T) {
	for _, platform := range []string{"github", "gitlab", "gitea"} {
		t.Run(platform, func(t *testing.T) {
			task := reviewerRunnerTickTask("review", model.TaskStatusPending)
			task.DedupeKey = platform + ":mr-review:42:21:head-b"
			prefix, err := reviewerDedupePrefix(task)
			if err != nil || prefix != platform+":mr-review:42:21:" {
				t.Fatalf("unexpected prefix: %q err=%v", prefix, err)
			}
		})
	}
}

func TestTickSerializesReviewerHeadsAcrossPlatforms(t *testing.T) {
	for _, platform := range []string{"github", "gitlab", "gitea"} {
		t.Run(platform, func(t *testing.T) {
			ctx := context.Background()
			tasks, runs := newTickRepositories(t)
			runner := &fakeReviewerRunnerCoordinator{reconcileResult: &ReviewerRunnerReconcileResult{
				Execution: &model.AgentExecution{Status: model.AgentExecutionStatusRunning},
			}}
			orch := New(tasks, runs, WithReviewerRunner(runner, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
			a := reviewerRunnerTickTask("head-a", model.TaskStatusRunning)
			a.DedupeKey = platform + ":mr-review:42:21:head-a"
			b := reviewerRunnerTickTask("head-b", model.TaskStatusPending)
			b.DedupeKey = platform + ":mr-review:42:21:head-b"
			for _, task := range []*model.Task{a, b} {
				if err := tasks.Create(ctx, task); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 2; i++ {
				result, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10})
				if err != nil || len(result.DispatchFailures) != 0 || len(result.DispatchedTasks) != 0 || runner.startCalls != 0 {
					t.Fatalf("head B must wait without failure: result=%#v starts=%d err=%v", result, runner.startCalls, err)
				}
			}
			stored, err := tasks.GetByTaskID(ctx, b.TaskID)
			if err != nil || stored.Status != model.TaskStatusPending || stored.CurrentRunID != nil || stored.AttemptCount != 0 {
				t.Fatalf("deferred head B must remain pending: task=%#v err=%v", stored, err)
			}
			activeRuns, err := runs.ListRunning(ctx, 10)
			if err != nil || len(activeRuns) != 0 {
				t.Fatalf("deferred head B must not create a run: runs=%#v err=%v", activeRuns, err)
			}
			a.Status = model.TaskStatusSucceeded
			if err := tasks.Update(ctx, a); err != nil {
				t.Fatal(err)
			}
			result, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10})
			if err != nil || len(result.DispatchFailures) != 0 || len(result.DispatchedTasks) != 1 || runner.startCalls != 1 {
				t.Fatalf("head B must start after A settles: result=%#v starts=%d err=%v", result, runner.startCalls, err)
			}
			if runner.startRequest.Task.TaskID != b.TaskID {
				t.Fatalf("wrong head dispatched: %#v", runner.startRequest.Task)
			}
			if _, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10}); err != nil {
				t.Fatal(err)
			}
			if _, err := orch.DispatchReviewerTask(ctx, b.TaskID); !errors.Is(err, repository.ErrTaskNotClaimable) {
				t.Fatalf("running head B must not start again: %v", err)
			}
			if runner.startCalls != 1 {
				t.Fatalf("head B started %d times", runner.startCalls)
			}
		})
	}
}

func TestTickOrdersPendingReviewerHeads(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	runner := &fakeReviewerRunnerCoordinator{}
	orch := New(tasks, runs, WithReviewerRunner(runner, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	for index, head := range []string{"a", "b", "c"} {
		task := reviewerRunnerTickTask("head-"+head, model.TaskStatusPending)
		task.DedupeKey = "github:mr-review:42:21:head-" + head
		task.UpdatedAt = time.Unix(int64(3-index), 0)
		if err := tasks.Create(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	result, err := orch.Tick(ctx, TickRequest{DispatchLimit: 1})
	if err != nil || len(result.DispatchFailures) != 0 || len(result.DispatchedTasks) != 1 || runner.startCalls != 1 {
		t.Fatalf("only the oldest pending head must start: result=%#v starts=%d err=%v", result, runner.startCalls, err)
	}
	if runner.startRequest.Task.TaskID != "head-a" {
		t.Fatalf("later pending head bypassed predecessor: %s", runner.startRequest.Task.TaskID)
	}
}

func TestReviewerDispatchDoesNotBlockOtherIdentities(t *testing.T) {
	for _, key := range []string{
		"github:mr-review:42:22:head-b",
		"github:mr-review:420:21:head-b",
		"gitlab:mr-review:42:21:head-b",
	} {
		t.Run(key, func(t *testing.T) {
			ctx := context.Background()
			tasks, runs := newTickRepositories(t)
			runner := &fakeReviewerRunnerCoordinator{}
			orch := New(tasks, runs, WithReviewerRunner(runner, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
			for index, dedupeKey := range []string{"github:mr-review:42:21:head-a", key} {
				status := model.TaskStatusRunning
				if index == 1 {
					status = model.TaskStatusPending
				}
				task := reviewerRunnerTickTask(fmt.Sprintf("head-%d", index), status)
				task.DedupeKey = dedupeKey
				if err := tasks.Create(ctx, task); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := orch.DispatchReviewerTask(ctx, "head-1"); err != nil || runner.startCalls != 1 {
				t.Fatalf("unrelated identity must start: starts=%d err=%v", runner.startCalls, err)
			}
		})
	}
}

func TestTickRetriesReviewerPredecessorBeyondDispatchWindow(t *testing.T) {
	for _, limit := range []int{1, 5} {
		for _, failStart := range []bool{false, true} {
			t.Run(fmt.Sprintf("limit_%d/fail_start_%t", limit, failStart), func(t *testing.T) {
				ctx := context.Background()
				tasks, runs := newTickRepositories(t)
				runner := &fakeReviewerRunnerCoordinator{reconcileResult: &ReviewerRunnerReconcileResult{
					Execution: &model.AgentExecution{Status: model.AgentExecutionStatusRunning},
				}}
				if failStart {
					runner.startErrors = []error{errors.New("runner temporarily unavailable"), nil}
				}
				orch := New(tasks, runs, WithReviewerRunner(runner, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
				a := reviewerRunnerTickTask("head-a", model.TaskStatusRunning)
				a.DedupeKey = "github:mr-review:42:21:head-a"
				if err := tasks.Create(ctx, a); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < limit+2; i++ {
					b := reviewerRunnerTickTask(fmt.Sprintf("head-b-%d", i), model.TaskStatusPending)
					b.DedupeKey = "github:mr-review:42:21:" + b.TaskID
					b.UpdatedAt = time.Unix(2, 0)
					if err := tasks.Create(ctx, b); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := orch.AdvanceTask(ctx, a.TaskID, model.TaskStatusRetryPending); err != nil {
					t.Fatal(err)
				}
				for tick := 0; tick < 3; tick++ {
					result, err := orch.Tick(ctx, TickRequest{DispatchLimit: limit})
					if err != nil {
						t.Fatal(err)
					}
					wantDispatched, wantFailures := 0, 0
					if tick == 0 && failStart {
						wantFailures = 1
					} else if tick == 0 || tick == 1 && failStart {
						wantDispatched = 1
					}
					if len(result.DispatchedTasks) != wantDispatched || len(result.DispatchFailures) != wantFailures {
						t.Fatalf("tick %d: retry predecessor must be reachable without same-tick retry: %#v", tick+1, result)
					}
				}
				wantStarts := 1
				if failStart {
					wantStarts = 2
				}
				if runner.startCalls != wantStarts || runner.startRequest.Task.TaskID != a.TaskID {
					t.Fatalf("only predecessor must start: calls=%d task=%s", runner.startCalls, runner.startRequest.Task.TaskID)
				}
				for i := 0; i < limit+2; i++ {
					b, err := tasks.GetByTaskID(ctx, fmt.Sprintf("head-b-%d", i))
					if err != nil || b.Status != model.TaskStatusPending || b.CurrentRunID != nil || b.AttemptCount != 0 {
						t.Fatalf("waiting head must remain unclaimed: task=%#v err=%v", b, err)
					}
				}
				if _, err := orch.AdvanceTask(ctx, a.TaskID, model.TaskStatusSucceeded); err != nil {
					t.Fatal(err)
				}
				result, err := orch.Tick(ctx, TickRequest{DispatchLimit: limit})
				if err != nil || len(result.DispatchedTasks) != 1 || len(result.DispatchFailures) != 0 || runner.startRequest.Task.TaskID != "head-b-0" {
					t.Fatalf("oldest waiting head must start after predecessor settles: result=%#v err=%v", result, err)
				}
				if _, err := orch.Tick(ctx, TickRequest{DispatchLimit: limit}); err != nil {
					t.Fatal(err)
				}
				if runner.startCalls != wantStarts+1 {
					t.Fatalf("waiting heads must not start concurrently or repeatedly: %d starts", runner.startCalls)
				}
			})
		}
	}
}

func TestTickDispatchesOtherMRBeyondDeferredReviewerWindow(t *testing.T) {
	for _, limit := range []int{1, 5} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			ctx := context.Background()
			tasks, runs := newTickRepositories(t)
			runner := &fakeReviewerRunnerCoordinator{}
			orch := New(tasks, runs, WithReviewerRunner(runner, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
			for i := 0; i < 2*limit+4; i++ {
				status := model.TaskStatusPending
				if i == 0 {
					status = model.TaskStatusRunning
				}
				task := reviewerRunnerTickTask(fmt.Sprintf("head-%d", i), status)
				task.DedupeKey = "gitlab:mr-review:42:21:" + task.TaskID
				task.UpdatedAt = time.Unix(2, 0)
				if i >= limit+3 {
					iid := i + 22
					task.DedupeKey = fmt.Sprintf("gitlab:mr-review:42:%d:%s", iid, task.TaskID)
					task.ContextJSON = strings.Replace(task.ContextJSON, `"iid":21`, fmt.Sprintf(`"iid":%d`, iid), 1)
				}
				if err := tasks.Create(ctx, task); err != nil {
					t.Fatal(err)
				}
			}
			result, err := orch.Tick(ctx, TickRequest{DispatchLimit: limit})
			if err != nil || len(result.DispatchedTasks) != limit || len(result.DispatchFailures) != 0 || runner.startCalls != limit {
				t.Fatalf("unrelated MRs must fill but not exceed the dispatch budget: result=%#v starts=%d err=%v", result, runner.startCalls, err)
			}
			for i, dispatched := range result.DispatchedTasks {
				if dispatched.TaskID != fmt.Sprintf("head-%d", limit+3+i) {
					t.Fatalf("wrong task dispatched: %#v", dispatched)
				}
			}
		})
	}
}

type arrivingReviewerTaskRepository struct {
	TaskRepository
	arrivals int
}

func (r *arrivingReviewerTaskRepository) HasActiveReviewerTaskForDedupePrefix(ctx context.Context, prefix, excludeTaskID string) (bool, error) {
	r.arrivals++
	if r.arrivals > 10 {
		return false, errors.New("tick kept absorbing newly arriving heads")
	}
	task := reviewerRunnerTickTask(fmt.Sprintf("arrival-%d", r.arrivals), model.TaskStatusPending)
	task.DedupeKey = prefix + task.TaskID
	if err := r.Create(ctx, task); err != nil {
		return false, err
	}
	return r.TaskRepository.HasActiveReviewerTaskForDedupePrefix(ctx, prefix, excludeTaskID)
}

func TestTickLeavesNewReviewerArrivalsForNextTick(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	for i, status := range []model.TaskStatus{model.TaskStatusRunning, model.TaskStatusPending} {
		task := reviewerRunnerTickTask(fmt.Sprintf("head-%d", i), status)
		task.DedupeKey = "github:mr-review:42:21:" + task.TaskID
		if err := tasks.Create(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	repo := &arrivingReviewerTaskRepository{TaskRepository: tasks}
	orch := New(repo, runs, WithReviewerRunner(&fakeReviewerRunnerCoordinator{}, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	result, err := orch.Tick(ctx, TickRequest{DispatchLimit: 1})
	if err != nil || len(result.DispatchFailures) != 0 || len(result.DispatchedTasks) != 0 || repo.arrivals != 1 {
		t.Fatalf("tick must stop at its initial candidate boundary: result=%#v arrivals=%d err=%v", result, repo.arrivals, err)
	}
}
