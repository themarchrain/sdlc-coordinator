package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentEnvironmentUsesPersistentToolCaches(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	t.Setenv("AGENT_RUNNER_CACHE_ROOT", root)
	t.Setenv("CODEX_HOME", "/cli-config")
	t.Setenv("PATH", "/custom-bin:/usr/bin")
	want := map[string]string{
		"AGENT_RUNNER_CACHE_ROOT": root,
		"GOPATH":                  filepath.Join(root, "go"),
		"GOMODCACHE":              filepath.Join(root, "go", "pkg", "mod"),
		"GOCACHE":                 filepath.Join(root, "go-build"),
		"npm_config_cache":        filepath.Join(root, "npm"),
		"NPM_CONFIG_CACHE":        filepath.Join(root, "npm"),
		"pnpm_config_store_dir":   filepath.Join(root, "pnpm"),
		"PNPM_CONFIG_STORE_DIR":   filepath.Join(root, "pnpm"),
		"PIP_CACHE_DIR":           filepath.Join(root, "pip"),
		"HOME":                    "/private-home", "XDG_CACHE_HOME": "/private-home/.cache",
		"CODEX_HOME": "/cli-config", "PATH": "/custom-bin:/usr/bin",
	}
	for name := range want {
		if name != "AGENT_RUNNER_CACHE_ROOT" && name != "CODEX_HOME" && name != "PATH" {
			t.Setenv(name, "/stale")
		}
	}
	got := map[string]string{}
	for _, entry := range agentEnvironment("/private-home") {
		key, value, _ := strings.Cut(entry, "=")
		got[key] = value
	}
	for name, value := range want {
		if got[name] != value {
			t.Errorf("%s = %q, want %q", name, got[name], value)
		}
	}
}

func TestLaunchReusesPersistentCachesAcrossInvocations(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to launch as Agent")
	}
	base, err := os.MkdirTemp("", "agent-cache-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	if err := os.Chmod(base, 0o755); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(base, "cache")
	if err := os.Mkdir(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(cache, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_RUNNER_CACHE_ROOT", cache)
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	shellData, err := os.ReadFile(shellPath)
	if err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(base, "sh")
	if err := os.WriteFile(shell, shellData, 0o755); err != nil {
		t.Fatal(err)
	}
	previous := allowedBinaries
	allowedBinaries = []string{shell}
	t.Cleanup(func() { allowedBinaries = previous })
	if err := os.MkdirAll(filepath.Dir(agentRunLockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "workspaces")
	for _, run := range []string{"first", "second"} {
		workspace := filepath.Join(root, "runs", run)
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			t.Fatal(err)
		}
		script := `set -eu
test "$(id -u)" = 10002
test "$(id -g)" = 10002
printf '%s\n' "$HOME" > home.path
for directory in "$GOCACHE" "$GOMODCACHE" "$npm_config_cache" "$pnpm_config_store_dir" "$PIP_CACHE_DIR"; do
  case "$directory" in "$AGENT_RUNNER_CACHE_ROOT"/*) ;; *) exit 23 ;; esac
  mkdir -p "$directory"
  if [ "$1" = first ]; then printf retained > "$directory/probe"; else test "$(cat "$directory/probe")" = retained; fi
done
if [ "$1" = second ]; then exit 42; fi
`
		code, err := launch([]string{"--workspace-root", root, "--workspace", workspace, "--", shell, "-c", script, "cache-probe", run})
		wantCode := 0
		if run == "second" {
			wantCode = 42
		}
		if err != nil || code != wantCode {
			t.Fatalf("%s launch = %d, %v; want %d", run, code, err, wantCode)
		}
		home, err := os.ReadFile(filepath.Join(workspace, "home.path"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(strings.TrimSpace(string(home))); !os.IsNotExist(err) {
			t.Fatalf("temporary HOME remains: %v", err)
		}
		assertOwner(t, workspace, runnerUID, runnerGID)
		if modeOf(t, workspace) != 0o700 {
			t.Fatal("run workspace is not private after exit")
		}
		if err := os.RemoveAll(workspace); err != nil {
			t.Fatal(err)
		}
	}
}
