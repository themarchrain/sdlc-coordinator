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
	result, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10})
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
