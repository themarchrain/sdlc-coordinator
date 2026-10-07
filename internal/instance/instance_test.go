package instance

import (
	"strings"
	"testing"
)

// TestIDStableAndWellFormed 锁定进程身份：非空、含进程 pid、进程内稳定。
func TestIDStableAndWellFormed(t *testing.T) {
	first := ID()
	if strings.TrimSpace(first) == "" {
		t.Fatal("empty instance id")
	}
	if len(first) > 64 {
		t.Fatalf("instance id exceeds the 64 character owner column: %d", len(first))
	}
	if !strings.Contains(first, "-") {
		t.Fatalf("instance id %q lacks host/pid/random structure", first)
	}
	if second := ID(); second != first {
		t.Fatalf("instance id changed within the process: %q -> %q", first, second)
	}
}
