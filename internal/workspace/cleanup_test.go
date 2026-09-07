package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveRunDirectoryDeletesOnlyThatRun(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "runs", "run-keep")
	remove := filepath.Join(root, "runs", "run-gone")
	if err := os.MkdirAll(filepath.Join(keep, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(remove, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRunDirectory(root, "run-gone"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(remove); !os.IsNotExist(err) {
		t.Fatalf("removed run still exists: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unrelated run was removed: %v", err)
	}
	if err := RemoveRunDirectory(root, "run-gone"); err != nil {
		t.Fatalf("missing directory must be idempotent: %v", err)
	}
}

func TestRemoveRunDirectoryRejectsUnsafeRunID(t *testing.T) {
	if err := RemoveRunDirectory(t.TempDir(), "../escape"); err == nil {
		t.Fatal("expected unsafe run id to be rejected")
	}
}
