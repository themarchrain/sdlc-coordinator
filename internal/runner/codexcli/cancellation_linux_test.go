//go:build linux

package codexcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCancellationWaitsForProcessTreeExit(t *testing.T) {
	t.Setenv("AGENT_RUNNER_CLI_LAUNCHER", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var pids struct {
		Parent int `json:"parent"`
		Child  int `json:"child"`
	}
	spec := commandSpec{Binary: "/bin/sh", Args: []string{"-c", `sleep 30 & child=$!; printf '{"parent":%s,"child":%s}\n' $$ $child; wait`}, Env: os.Environ()}
	err := newProcessRunner().Run(ctx, spec, func(line []byte) error {
		if err := json.Unmarshal(line, &pids); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if err == nil || pids.Parent <= 0 || pids.Child <= 0 {
		t.Fatalf("cancellation missing: pids=%+v err=%v", pids, err)
	}
	for _, pid := range []int{pids.Parent, pids.Child} {
		deadline := time.Now().Add(3 * time.Second)
		for {
			raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
			// 进程在 open 与 read 之间被回收时 procfs 的 read 返回 ESRCH
			// 而非 ENOENT,同样视为已退出。
			if os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			fields := strings.Fields(string(raw)[strings.LastIndex(string(raw), ")")+1:])
			if len(fields) > 0 && fields[0] == "Z" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal(fmt.Sprintf("process %d still running: %s", pid, raw))
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Logf("process %d exited", pid)
	}
}
