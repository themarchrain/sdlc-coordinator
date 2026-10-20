//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMCPDeliveryWhileAgentOwnsWorkspace(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("UID isolation requires root")
	}
	for _, binary := range []string{"git", "setcap"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("%s is required for the isolated delivery test", binary)
		}
	}
	base, err := os.MkdirTemp("", "mcp-delivery-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	if err := os.Chmod(base, 0o755); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	cli, helper := filepath.Join(base, "cli"), filepath.Join(base, "launcher")
	for _, path := range []string{cli, helper} {
		if err := os.WriteFile(path, data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chown(helper, 0, runnerGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(helper, 0o750); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("setcap", "cap_chown,cap_dac_override,cap_fowner,cap_kill,cap_setgid,cap_setuid=ep", helper).CombinedOutput(); err != nil {
		t.Fatalf("set launcher capabilities: %v: %s", err, output)
	}
	workspaceRoot := filepath.Join(base, "workspaces")
	runRoot := filepath.Join(workspaceRoot, "runs", "run-mcp")
	repo, origin := filepath.Join(runRoot, "repo"), filepath.Join(base, "origin.git")
	for _, path := range []string{repo, origin, filepath.Dir(agentRunLockPath)} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chown(filepath.Dir(agentRunLockPath), runnerUID, runnerGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(agentRunLockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	runDeliveryGit(t, origin, "init", "--bare")
	runDeliveryGit(t, repo, "init")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("baseline\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runDeliveryGit(t, repo, "add", "-A")
	runDeliveryGit(t, repo, "commit", "-m", "baseline")
	if err := os.WriteFile(filepath.Join(runRoot, "workspace-manifest.json"), []byte(`{"run_id":"run-mcp"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := chownTree(workspaceRoot, runnerUID, runnerGID); err != nil {
		t.Fatal(err)
	}
	if err := chownTree(origin, runnerUID, runnerGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(workspaceRoot, "runs", "other")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(other, runnerUID, runnerGID); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SDLC_DELIVERY_ROOT", runRoot)
	t.Setenv("SDLC_DELIVERY_ORIGIN", origin)
	t.Setenv("SDLC_DELIVERY_OTHER", other)
	t.Setenv("SDLC_DELIVERY_HELPER", "agent")
	command := func(mode string) *exec.Cmd {
		binary := cli
		if mode == "share" {
			binary = helper
		}
		cmd := exec.Command(binary, "-test.run=^TestDeliveryProcessHelper$")
		cmd.Env = append(os.Environ(), "SDLC_DELIVERY_HELPER="+mode)
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: runnerUID, Gid: runnerGID}}
		return cmd
	}
	deliveryErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil || call.Method != "tools/call" || call.Params.Name != "submit_developer_result" {
			http.Error(w, "invalid MCP call", http.StatusBadRequest)
			return
		}
		if output, err := command("probe").CombinedOutput(); err == nil || !strings.Contains(string(output), "permission denied") {
			failure := fmt.Errorf("expected the original Runner permission failure before access grant: %v: %s", err, output)
			deliveryErr <- failure
			http.Error(w, failure.Error(), http.StatusInternalServerError)
			return
		}
		homes, err := filepath.Glob(filepath.Join(runRoot, ".agent-home-*"))
		if err != nil || len(homes) != 1 {
			deliveryErr <- fmt.Errorf("locate active Agent home: %v: %v", homes, err)
			http.Error(w, "missing Agent home", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(filepath.Join(homes[0], "private-runtime-file"), []byte("runtime"), 0o600); err != nil {
			deliveryErr <- err
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, mode := range []string{"share", "deliver", "share"} {
			if output, err := command(mode).CombinedOutput(); err != nil {
				failure := fmt.Errorf("%s during MCP call: %w: %s", mode, err, output)
				deliveryErr <- failure
				http.Error(w, failure.Error(), http.StatusInternalServerError)
				return
			}
		}
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"status\":\"accepted\",\"task_id\":\"task-mcp\",\"run_id\":\"run-mcp\",\"run_status\":\"succeeded\"}"}],"isError":false}}`)
	}))
	defer server.Close()
	t.Setenv("SDLC_DELIVERY_MCP", server.URL)
	previous := allowedBinaries
	allowedBinaries = []string{cli}
	t.Cleanup(func() { allowedBinaries = previous })
	code, err := launch([]string{"--workspace-root", workspaceRoot, "--workspace", runRoot, "--", cli, "-test.run=^TestDeliveryProcessHelper$"})
	select {
	case delivery := <-deliveryErr:
		t.Fatal(delivery)
	default:
	}
	if err != nil || code != 0 {
		t.Fatalf("Agent MCP delivery = %d, %v", code, err)
	}
	assertOwner(t, runRoot, runnerUID, runnerGID)
	if modeOf(t, runRoot) != 0o700 {
		t.Fatal("completed run must be private to Runner")
	}
	assertOwner(t, other, runnerUID, runnerGID)
	runDeliveryGit(t, origin, "rev-parse", "refs/heads/delivery")
}

func TestDeliveryProcessHelper(t *testing.T) {
	mode := os.Getenv("SDLC_DELIVERY_HELPER")
	if mode == "" {
		return
	}
	root := os.Getenv("SDLC_DELIVERY_ROOT")
	switch mode {
	case "share":
		os.Args = []string{"launcher", "--share-workspace", filepath.Dir(filepath.Dir(root)), root}
		main()
	case "probe", "deliver":
		if _, err := os.ReadFile(filepath.Join(root, "workspace-manifest.json")); err != nil {
			t.Fatalf("Runner cannot read manifest while Agent waits for MCP: %v", err)
		}
		if mode == "probe" {
			return
		}
		repo := filepath.Join(root, "repo")
		runDeliveryGit(t, repo, "add", "-A")
		runDeliveryGit(t, repo, "commit", "-m", "fix: deliver through MCP")
		runDeliveryGit(t, repo, "push", os.Getenv("SDLC_DELIVERY_ORIGIN"), "HEAD:refs/heads/delivery")
	case "agent":
		if os.Getuid() != agentUID || os.Getgid() != agentGID {
			t.Fatal("Agent identity changed")
		}
		for _, path := range []string{os.Getenv("SDLC_DELIVERY_OTHER"), filepath.Dir(agentRunLockPath)} {
			if _, err := os.ReadDir(path); !os.IsPermission(err) {
				t.Fatalf("Agent must not access %s: %v", path, err)
			}
		}
		file := filepath.Join(root, "repo", "new-private-file")
		if err := os.WriteFile(file, []byte("change\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		client := http.Client{Timeout: 15 * time.Second}
		response, err := client.Post(os.Getenv("SDLC_DELIVERY_MCP"), "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"submit_developer_result","arguments":{"outcome":"change_ready"}}}`))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		var receipt struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		if response.StatusCode != http.StatusOK || json.Unmarshal(body, &receipt) != nil || receipt.Result.IsError || len(receipt.Result.Content) != 1 || !strings.Contains(receipt.Result.Content[0].Text, `"status":"accepted"`) {
			t.Fatalf("MCP submission failed: %s", body)
		}
		if err := os.WriteFile(file, []byte("change\n"), 0o600); err != nil {
			t.Fatalf("Agent lost its workspace ownership: %v", err)
		}
		if modeOf(t, os.Getenv("HOME")) != 0o700 {
			t.Fatal("Agent home permissions changed during delivery")
		}
	}
}

func TestRecoverWorkspaceAfterMCPAccessGrant(t *testing.T) {
	workspaceRoot, runRoot := newRecoveryWorkspace(t)
	if err := os.Chown(runRoot, agentUID, runnerGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runRoot, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := recoverRunWorkspaces(workspaceRoot, filepath.Join(t.TempDir(), "agent-cli.lock")); err != nil {
		t.Fatal(err)
	}
	assertOwner(t, runRoot, runnerUID, runnerGID)
	if modeOf(t, runRoot) != 0o700 {
		t.Fatal("recovered run must be private to Runner")
	}
}

func TestRunnerAccessDoesNotFollowLinksOutsideWorkspace(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing ownership requires root")
	}
	directory := t.TempDir()
	outside := t.TempDir()
	file := filepath.Join(outside, "private")
	if err := os.WriteFile(file, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(file, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	root, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for name, target := range map[string]string{"linked-directory": outside, "linked-file": file} {
		if err := os.Symlink(target, filepath.Join(directory, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, relative := range []string{"linked-directory/private", "linked-file"} {
		if err := grantRunnerGroupAccess(root, relative, recoveryEntry{}); err == nil {
			t.Fatalf("access grant followed %s", relative)
		}
	}
	if err := os.Link(file, filepath.Join(directory, "hard-linked")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if err := grantRunnerGroupAccess(root, "hard-linked", recoveryEntry{device: stat.Dev, inode: stat.Ino}); err != nil {
		t.Fatal(err)
	}
	assertOwner(t, file, agentUID, agentGID)
	if modeOf(t, file) != 0o600 {
		t.Fatal("access grant changed a link target outside the workspace")
	}
}

func TestRunnerAccessRejectsDirectorySubstitution(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing ownership requires root")
	}
	directory := t.TempDir()
	repo, home := filepath.Join(directory, "repo"), filepath.Join(directory, ".agent-home-private")
	for _, path := range []string{repo, home} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "private"), []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := chownTree(directory, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := inspectRunWorkspaceTree(directory, info.Sys().(*syscall.Stat_t).Dev)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(repo, filepath.Join(directory, "original-repo")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(home, repo); err != nil {
		t.Fatal(err)
	}
	root, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, entry := range entries {
		if entry.path != filepath.Join(repo, "private") {
			continue
		}
		if err := grantRunnerGroupAccess(root, "repo/private", entry); err == nil {
			t.Fatal("access grant accepted a substituted private directory")
		}
		assertOwner(t, filepath.Join(repo, "private"), agentUID, agentGID)
		if modeOf(t, filepath.Join(repo, "private")) != 0o600 {
			t.Fatal("access grant changed the substituted private file")
		}
		return
	}
	t.Fatal("fixture did not preflight the repository file")
}

func runDeliveryGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	config := []string{"-c", "safe.directory=*", "-c", "commit.gpgsign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test"}
	command := exec.Command("git", append(config, args...)...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
