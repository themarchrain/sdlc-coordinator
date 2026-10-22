package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"sdlc-coordinator/internal/runner"
)

func TestCancelBeforeCreatePersistsTombstone(t *testing.T) {
	root := t.TempDir()
	service, err := runner.NewService(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := sampleRequest()
	ack, err := service.CancelDurable(request.ExecutionID, runner.CancellationRequest{StopID: "stop-one", TaskID: request.TaskID})
	if err != nil || !ack.CancelIntentPersisted || !ack.FutureCreateBlocked || ack.ExecutionPresent || ack.ExitConfirmed || ack.Execution != nil {
		t.Fatalf("invalid no-execution proof: %+v err=%v", ack, err)
	}
	if _, err := service.Get(request.ExecutionID); !errors.Is(err, runner.ErrNotFound) {
		t.Fatalf("cancellation fabricated execution: %v", err)
	}
	if _, err := service.Create(context.Background(), request); err == nil {
		t.Fatal("late create accepted")
	}
}

func TestRestartLoadsCancellationFirst(t *testing.T) {
	root := t.TempDir()
	service, _ := runner.NewService(root, nil)
	request := sampleRequest()
	if _, err := service.CancelDurable(request.ExecutionID, runner.CancellationRequest{StopID: "stop-one", TaskID: request.TaskID}); err != nil {
		t.Fatal(err)
	}
	restarted, err := runner.NewService(root, map[string]runner.Provider{"fake": &successfulProvider{result: json.RawMessage(`{}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Create(context.Background(), request); err == nil {
		t.Fatal("restart forgot tombstone")
	}
}

func TestDuplicateCancel(t *testing.T) {
	service, _ := runner.NewService(t.TempDir(), nil)
	request := runner.CancellationRequest{StopID: "stop-one", TaskID: "task"}
	for i := 0; i < 2; i++ {
		ack, err := service.CancelDurable("run", request)
		if err != nil || ack.StopID != request.StopID || !ack.FutureCreateBlocked {
			t.Fatalf("duplicate: %+v %v", ack, err)
		}
	}
}

func TestCancellationRejectsTaskIdentityConflict(t *testing.T) {
	service, _ := runner.NewService(t.TempDir(), nil)
	if _, err := service.CancelDurable("run", runner.CancellationRequest{StopID: "stop-one", TaskID: "task"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CancelDurable("run", runner.CancellationRequest{StopID: "stop-one", TaskID: "other"}); !errors.Is(err, runner.ErrConflict) {
		t.Fatalf("conflicting task accepted: %v", err)
	}
	for _, id := range []string{"../escape", "", "a/b"} {
		if _, err := service.CancelDurable(id, runner.CancellationRequest{StopID: "stop-one", TaskID: "task"}); err == nil {
			t.Fatalf("unsafe target accepted: %s", id)
		}
	}
}

type ignoresCancellationProvider struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (p *ignoresCancellationProvider) Execute(context.Context, runner.ProviderRequest, runner.EmitFunc) (json.RawMessage, error) {
	p.calls.Add(1)
	close(p.started)
	<-p.release
	return json.RawMessage(`{"late":true}`), nil
}

func TestCancelledProviderCannotPublishLateSuccess(t *testing.T) {
	p := &ignoresCancellationProvider{started: make(chan struct{}), release: make(chan struct{})}
	service, _ := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": p})
	request := sampleRequest()
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	<-p.started
	ack, err := service.CancelDurable(request.ExecutionID, runner.CancellationRequest{StopID: "stop-one", TaskID: request.TaskID})
	if err != nil || ack.ExitConfirmed {
		close(p.release)
		t.Fatalf("premature exit proof: %+v %v", ack, err)
	}
	close(p.release)
	waitForStatus(t, service, request.ExecutionID, runner.StatusCancelled)
	ack, err = service.CancelDurable(request.ExecutionID, runner.CancellationRequest{StopID: "stop-one", TaskID: request.TaskID})
	if err != nil || !ack.ExitConfirmed {
		t.Fatalf("missing exit proof: %+v %v", ack, err)
	}
	if _, err := service.Result(request.ExecutionID); err == nil {
		t.Fatal("cancelled provider published late result")
	}
}

func TestCancelCreateRace(t *testing.T) {
	for i := 0; i < 10; i++ {
		p := &blockingProvider{started: make(chan struct{})}
		service, _ := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": p})
		request := sampleRequest()
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _, _ = service.Create(context.Background(), request) }()
		go func() {
			defer wg.Done()
			<-start
			_, err := service.CancelDurable(request.ExecutionID, runner.CancellationRequest{StopID: "stop-one", TaskID: request.TaskID})
			if err != nil {
				t.Errorf("cancel: %v", err)
			}
		}()
		close(start)
		wg.Wait()
		if _, err := service.Get(request.ExecutionID); err == nil {
			waitForStatus(t, service, request.ExecutionID, runner.StatusCancelled)
		}
		if _, err := service.Create(context.Background(), request); err == nil {
			t.Fatal("post-cancel create accepted")
		}
	}
}

func TestCancellationTombstoneSurvivesSuccessfulArtifactDeletion(t *testing.T) {
	root := t.TempDir()
	service, err := runner.NewService(root, map[string]runner.Provider{"fake": &successfulProvider{result: json.RawMessage(`{}`)}})
	if err != nil {
		t.Fatal(err)
	}
	request := sampleRequest()
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, service, request.ExecutionID, runner.StatusSucceeded)
	ack, err := service.CancelDurable(request.ExecutionID, runner.CancellationRequest{StopID: "stop-one", TaskID: request.TaskID})
	if err != nil || !ack.ExitConfirmed || ack.Execution.Status != runner.StatusSucceeded {
		t.Fatalf("success was not preserved: %+v %v", ack, err)
	}
	if err := service.Delete(request.ExecutionID); err != nil {
		t.Fatal(err)
	}
	restarted, err := runner.NewService(root, map[string]runner.Provider{"fake": &successfulProvider{result: json.RawMessage(`{}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Create(context.Background(), request); err == nil {
		t.Fatal("artifact deletion deleted cancellation authority")
	}
}

func TestCancellationPersistenceFailureCannotAcknowledge(t *testing.T) {
	root := t.TempDir()
	service, err := runner.NewService(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "cancellations")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cancellations"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	ack, err := service.CancelDurable("run", runner.CancellationRequest{StopID: "stop-one", TaskID: "task"})
	if err == nil || ack != nil {
		t.Fatalf("unpersisted cancellation acknowledged: %+v %v", ack, err)
	}
}
