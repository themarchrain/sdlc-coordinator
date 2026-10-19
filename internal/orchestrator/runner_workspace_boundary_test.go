package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerDispatchRequiresTaskBundlePublisher(t *testing.T) {
	t.Run("reviewer", func(t *testing.T) {
		f := newReviewerRunnerFixture(t)
		f.coordinator.SetTaskBundlePublisher(nil)
		_, err := f.coordinator.Start(context.Background(), f.startRequest())
		if err == nil || !strings.Contains(err.Error(), "publisher") {
			t.Fatalf("dispatch without a publisher must fail, got %v", err)
		}
		if f.preparer.calls != 0 || f.runner.createCalls != 0 {
			t.Fatal("missing publisher must fail before workspace preparation or execution")
		}
	})
	t.Run("developer", func(t *testing.T) {
		f := newDeveloperRunnerFixture(t, "develop_from_issue")
		f.coordinator.SetTaskBundlePublisher(nil)
		_, err := f.coordinator.Start(context.Background(), f.startRequest())
		if err == nil || !strings.Contains(err.Error(), "publisher") {
			t.Fatalf("dispatch without a publisher must fail, got %v", err)
		}
		if f.preparer.calls != 0 || f.runner.createCalls != 0 {
			t.Fatal("missing publisher must fail before workspace preparation or execution")
		}
	})
	t.Run("tester", func(t *testing.T) {
		f := newTesterIngressFixture(t)
		f.tester.SetTaskBundlePublisher(nil)
		record := f.records["TesterWorker"]
		root := t.TempDir()
		_, err := f.tester.Start(context.Background(), testerIngressStartRequest(record.task, record.run, root))
		if err == nil || !strings.Contains(err.Error(), "publisher") {
			t.Fatalf("dispatch without a publisher must fail, got %v", err)
		}
		if f.testerClient.createCalls != 0 {
			t.Fatal("missing publisher must fail before execution")
		}
		if _, err := os.Stat(filepath.Join(root, "runs", record.run.RunID)); !os.IsNotExist(err) {
			t.Fatalf("coordinator must not create a local workspace: %v", err)
		}
	})
}

func TestCleanupWithoutRunnerCleanerDoesNotDeleteLocalWorkspace(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	f := newArtifactCleanupFixture(t, root)
	f.orch.runWorkspaceCleaner = nil
	seedFullySucceededRun(t, ctx, f, root, "run-no-cleaner", f.now.Add(-8*24*time.Hour))
	if _, err := f.orch.Tick(ctx, TickRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "runs", "run-no-cleaner")); err != nil {
		t.Fatalf("coordinator must not delete a local workspace: %v", err)
	}
	if len(f.store.deleted) != 0 {
		t.Fatal("execution artifacts cannot be purged without the Runner workspace cleaner")
	}
	run, err := f.runs.GetByRunID(ctx, "run-no-cleaner")
	if err != nil {
		t.Fatal(err)
	}
	if run.ArtifactsPurgedAt != nil || run.ArtifactsPurgeStartedAt != nil {
		t.Fatal("cleanup without a Runner cleaner must not claim or complete the purge")
	}
}
