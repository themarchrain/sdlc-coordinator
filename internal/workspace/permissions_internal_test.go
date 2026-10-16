package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRestrictRunWorkspaceRootMakesOnlyRunBoundaryPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not portable on windows; behavior covered by posix runs")
	}
	root := t.TempDir()
	nested := filepath.Join(root, "repo", ".git", "objects")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("setup nested dir: %v", err)
	}
	looseFile := filepath.Join(root, "repo", "prompt.md")
	if err := os.WriteFile(looseFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup loose file: %v", err)
	}
	packedObject := filepath.Join(nested, "pack-1.pack")
	if err := os.WriteFile(packedObject, []byte("x"), 0o400); err != nil {
		t.Fatalf("setup packed object: %v", err)
	}
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatalf("make run root public: %v", err)
	}

	if err := restrictRunWorkspaceRoot(root); err != nil {
		t.Fatalf("restrict run workspace root: %v", err)
	}

	assertMode(t, root, 0o700)
	assertMode(t, filepath.Join(root, "repo"), 0o700)
	assertMode(t, nested, 0o700)
	assertMode(t, looseFile, 0o600)
	assertMode(t, packedObject, 0o400)
}

func TestRestrictRunWorkspaceRootRejectsMissingOrSymlinkedRoot(t *testing.T) {
	if err := restrictRunWorkspaceRoot(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected error for missing root")
	}
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "run")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	if err := restrictRunWorkspaceRoot(link); err == nil {
		t.Fatal("expected error for symlinked run root")
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s: expected mode %o, got %o", path, want, got)
	}
}
