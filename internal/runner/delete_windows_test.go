package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"sdlc-coordinator/internal/runner"
	"testing"
)

func TestCancelledArtifactDeletionCanRetryAfterFilesystemFailure(t *testing.T) {
	root := t.TempDir()
	p := &ignoresCancellationProvider{started: make(chan struct{}), release: make(chan struct{})}
	service, err := runner.NewService(root, map[string]runner.Provider{"fake": p})
	if err != nil {
		t.Fatal(err)
	}
	request := sampleRequest()
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	<-p.started
	if _, err := service.CancelDurable(request.ExecutionID, runner.CancellationRequest{StopID: "stop", TaskID: request.TaskID}); err != nil {
		t.Fatal(err)
	}
	close(p.release)
	waitForStatus(t, service, request.ExecutionID, runner.StatusCancelled)
	locked, err := os.OpenFile(filepath.Join(root, "executions", request.ExecutionID, "locked"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	if err := service.Delete(request.ExecutionID); err == nil {
		t.Fatal("fixture did not block RemoveAll")
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(request.ExecutionID); err != nil {
		t.Fatalf("delete cannot recover: %v", err)
	}
}
