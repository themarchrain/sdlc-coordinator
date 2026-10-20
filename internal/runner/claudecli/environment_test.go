package claudecli

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/runner"
)

func TestProviderPreservesToolchainEnvironment(t *testing.T) {
	want := map[string]string{
		"AGENT_RUNNER_CACHE_ROOT": "/app/custom-cache",
		"JAVA_HOME":               "/usr/lib/jvm/java-21-openjdk",
		"JAVA8_HOME":              "/usr/lib/jvm/java-1.8-openjdk",
		"JAVA17_HOME":             "/usr/lib/jvm/java-17-openjdk",
		"JAVA21_HOME":             "/usr/lib/jvm/java-21-openjdk",
		"MAVEN_OPTS":              "-Daether.syncContext.named.factory=file-lock -Daether.syncContext.named.nameMapper=file-gav",
		"GOPROXY":                 "https://proxy.example.test,direct",
	}
	for name, value := range want {
		t.Setenv(name, value)
	}
	for _, name := range []string{"AGENT_RUNNER_INTERNAL_TOKEN", "CONTROL_PLANE_DSN", "GITLAB_TOKEN"} {
		t.Setenv(name, "must-not-reach-agent")
	}
	root := t.TempDir()
	writeClaudeTestFile(t, root, "prompt.md", "review")
	writeClaudeTestFile(t, root, "schema.json", `{"type":"object","properties":{"verdict":{"type":"string"}},"required":["verdict"]}`)
	command := &fakeCommandRunner{lines: [][]byte{[]byte(`{"type":"result","subtype":"success","is_error":false,"structured_output":{"verdict":"approved"}}`)}}
	_, err := newProvider("test-cli", command).Execute(context.Background(), runner.ProviderRequest{
		WorkspacePath: root, PromptFile: "prompt.md", SchemaFile: "schema.json",
	}, func(string, map[string]any) {})
	if err != nil {
		t.Fatal(err)
	}
	got := environmentMap(command.spec.Env)
	for name, value := range want {
		if got[name] != value {
			t.Errorf("child %s = %q, want %q", name, got[name], value)
		}
	}
	for _, name := range []string{"AGENT_RUNNER_INTERNAL_TOKEN", "CONTROL_PLANE_DSN", "GITLAB_TOKEN"} {
		if _, exists := got[name]; exists {
			t.Errorf("unapproved %s reached child environment", name)
		}
	}
}
