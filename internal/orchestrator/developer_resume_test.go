package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func openDeveloperResumeOrchestrator(t *testing.T, objects RecoveryObjectReader) (*Orchestrator, *repository.TaskRepository, *model.Task) {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatal(err)
	}
	tasks := repository.NewTaskRepository(db)
	ctx := context.Background()
	previousRunID := "run-human-required"
	task := &model.Task{
		TaskID: "task-developer-resume", DedupeKey: "gitlab:issue-develop:1001:19",
		Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusHumanRequired,
		TargetRepo: "demo-group/demo-project", CurrentRunID: &previousRunID,
		LastError:   "需要产品确认",
		ContextJSON: `{"project":{"path":"demo-group/demo-project"},"issue":{"iid":19}}`,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	options := []Option{}
	if objects != nil {
		options = append(options, WithRecoveryObjectReader(objects))
	}
	return New(tasks, repository.NewRunRepository(db), options...), tasks, task
}

func developerResumeRequest(task *model.Task) model.DeveloperResumeRequest {
	return model.DeveloperResumeRequest{
		DedupeKey: task.DedupeKey, NoteID: 7001, ResumeContext: "按确认方案继续实现", AssigneeIDs: []int{42},
	}
}

func TestResumeDeveloperTaskRefusesAClosedIssue(t *testing.T) {
	orch, tasks, task := openDeveloperResumeOrchestrator(t, fakeRecoveryObjectReader{object: RecoveryObjectState{State: codehost.MergeRequestStateClosed}})
	_, err := orch.ResumeDeveloperTask(context.Background(), developerResumeRequest(task))
	if !errors.Is(err, model.ErrTaskObjectNotOpen) {
		t.Fatalf("resume error = %v, want the object gate", err)
	}
	current, err := tasks.GetByTaskID(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != model.TaskStatusHumanRequired || current.CurrentRunID == nil {
		t.Fatalf("refused resume changed the task: %+v", current)
	}
}

func TestResumeDeveloperTaskFailsClosedWhenTheObjectIsUnreadable(t *testing.T) {
	for name, reader := range map[string]RecoveryObjectReader{
		"reader error": fakeRecoveryObjectReader{err: errors.New("code host unreachable")},
		"no reader":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			orch, tasks, task := openDeveloperResumeOrchestrator(t, reader)
			_, err := orch.ResumeDeveloperTask(context.Background(), developerResumeRequest(task))
			if !errors.Is(err, model.ErrTaskObjectUnavailable) {
				t.Fatalf("resume error = %v, want the unavailable gate", err)
			}
			current, err := tasks.GetByTaskID(context.Background(), task.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Status != model.TaskStatusHumanRequired {
				t.Fatalf("unreadable object still resumed the task: %+v", current)
			}
		})
	}
}

// deadlineObjectReader records the deadline it was given and answers like an
// unresponsive code host, so the guard has to impose the bound itself.
type deadlineObjectReader struct {
	deadline time.Duration
	called   bool
}

func (r *deadlineObjectReader) ReadRecoveryObject(ctx context.Context, _ *model.Task) (RecoveryObjectState, error) {
	r.called = true
	until, ok := ctx.Deadline()
	if !ok {
		return RecoveryObjectState{}, errors.New("object read was given no deadline")
	}
	r.deadline = time.Until(until)
	return RecoveryObjectState{}, context.DeadlineExceeded
}

func TestResumeDeveloperTaskBoundsTheObjectRead(t *testing.T) {
	reader := &deadlineObjectReader{}
	orchestrator, _, task := openDeveloperResumeOrchestrator(t, reader)
	_, err := orchestrator.ResumeDeveloperTask(context.Background(), developerResumeRequest(task))
	if !errors.Is(err, model.ErrTaskObjectUnavailable) {
		t.Fatalf("resume error = %v, want the unavailable gate", err)
	}
	if !reader.called || reader.deadline <= 0 || reader.deadline > 10*time.Second {
		t.Fatalf("object read deadline = %v (called=%v)", reader.deadline, reader.called)
	}
}

func TestResumeDeveloperTaskResumesWhileTheIssueIsOpen(t *testing.T) {
	orch, tasks, task := openDeveloperResumeOrchestrator(t, fakeRecoveryObjectReader{object: RecoveryObjectState{State: codehost.MergeRequestStateOpen}})
	resumed, err := orch.ResumeDeveloperTask(context.Background(), developerResumeRequest(task))
	if err != nil {
		t.Fatalf("resume open issue: %v", err)
	}
	if resumed.Status != model.TaskStatusPending || resumed.CurrentRunID != nil {
		t.Fatalf("resumed task state = %+v", resumed)
	}
	current, err := tasks.GetByTaskID(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != model.TaskStatusPending {
		t.Fatalf("resume did not reach pending: %+v", current)
	}
}
