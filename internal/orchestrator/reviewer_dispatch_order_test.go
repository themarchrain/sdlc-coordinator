package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
)

func TestDispatchReviewerTaskDefersToActivePredecessorOnSameMR(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	orch := New(tasks, runs, WithReviewerRunner(&fakeReviewerRunnerCoordinator{}, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))

	earlier := reviewerRunnerTickTask("task-mr-21-earlier", model.TaskStatusRunning)
	earlier.DedupeKey = "gitlab:mr-review:group/project:21:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := tasks.Create(ctx, earlier); err != nil {
		t.Fatalf("create earlier review: %v", err)
	}
	later := reviewerRunnerTickTask("task-mr-21-later", model.TaskStatusPending)
	later.DedupeKey = "gitlab:mr-review:group/project:21:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := tasks.Create(ctx, later); err != nil {
		t.Fatalf("create later review: %v", err)
	}
	otherMR := reviewerRunnerTickTask("task-mr-22-other", model.TaskStatusPending)
	otherMR.DedupeKey = "gitlab:mr-review:group/project:22:cccccccccccccccccccccccccccccccc"
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
