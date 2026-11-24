//go:build linux

package picli

import (
	"reflect"
	"testing"
)

func TestCommandInvocationUsesAgentLauncher(t *testing.T) {
	t.Setenv("AGENT_RUNNER_CLI_LAUNCHER", cliLauncherPath)
	t.Setenv("AGENT_RUNNER_WORKSPACE_ROOT", "/app/workspaces")
	spec := commandSpec{
		Binary: "/opt/pi/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js",
		Args:   []string{"-p"}, Dir: "/app/workspaces/runs/run-1",
	}
	binary, args, launcher, err := commandInvocation(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--workspace-root", "/app/workspaces", "--workspace", spec.Dir, "--", spec.Binary, "-p"}
	if binary != cliLauncherPath || !launcher || !reflect.DeepEqual(args, want) {
		t.Fatalf("commandInvocation = (%q, %#v, %v), want (%q, %#v, true)", binary, args, launcher, cliLauncherPath, want)
	}
}
