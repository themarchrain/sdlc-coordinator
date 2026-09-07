package claudecli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProcessRunnerStreamsLinesAndForwardsStdin(t *testing.T) {
	runner := newProcessRunner()
	var lines []string
	err := runner.Run(context.Background(), helperCommandSpec("success", "prompt through stdin"), func(line []byte) error {
		lines = append(lines, string(line))
		return nil
	})
	if err != nil {
		t.Fatalf("run helper process: %v", err)
	}
	if len(lines) != 1 || lines[0] != `{"type":"result","structured_output":{"prompt":"prompt through stdin"}}` {
		t.Fatalf("unexpected streamed lines: %#v", lines)
	}
}

func TestProcessRunnerReturnsBoundedStderrForNonZeroExit(t *testing.T) {
	runner := newProcessRunner()
	err := runner.Run(context.Background(), helperCommandSpec("failure", ""), func([]byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "helper failed") || !strings.Contains(err.Error(), "exit") {
		t.Fatalf("unexpected process failure: %v", err)
	}
	if len(err.Error()) > maxProcessErrorBytes+1024 {
		t.Fatalf("process error was not bounded: %d bytes", len(err.Error()))
	}
}

func TestProcessRunnerCancelsLongRunningCommand(t *testing.T) {
	runner := newProcessRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := runner.Run(ctx, helperCommandSpec("wait", ""), func([]byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("cancelled command took too long: %s", time.Since(started))
	}
}

func TestClaudeCommandHelper(t *testing.T) {
	if os.Getenv("GO_WANT_CLAUDE_HELPER_PROCESS") != "1" {
		return
	}
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(2)
	}
	switch os.Args[separator+1] {
	case "success":
		raw, _ := io.ReadAll(os.Stdin)
		fmt.Printf(`{"type":"result","structured_output":{"prompt":%q}}`+"\n", strings.TrimSpace(string(raw)))
		os.Exit(0)
	case "failure":
		fmt.Fprintln(os.Stderr, "helper failed")
		os.Exit(7)
	case "wait":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func helperCommandSpec(mode, stdin string) commandSpec {
	return commandSpec{
		Binary: os.Args[0],
		Args:   []string{"-test.run=TestClaudeCommandHelper", "--", mode},
		Env:    append(os.Environ(), "GO_WANT_CLAUDE_HELPER_PROCESS=1"),
		Stdin:  stdin,
	}
}
