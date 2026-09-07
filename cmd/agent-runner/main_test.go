package main

import (
	"reflect"
	"testing"
)

func TestLoadConfigUsesRunnerDefaults(t *testing.T) {
	config := loadConfig(func(string) string { return "" })
	if config.HTTPAddr != ":8081" || config.DataRoot != "/app/runner-data" || config.ClaudeBinary != "" || config.CodexBinary != "" {
		t.Fatalf("unexpected runner defaults: %#v", config)
	}
}

func TestLoadConfigOnlyContainsServiceBootstrapSettings(t *testing.T) {
	values := map[string]string{
		"AGENT_RUNNER_HTTP_ADDR":     "127.0.0.1:19081",
		"AGENT_RUNNER_DATA_ROOT":     "/tmp/runner-data",
		"AGENT_RUNNER_CLAUDE_BINARY": "/home/runner/.local/bin/claude",
		"AGENT_RUNNER_CODEX_BINARY":  "/home/runner/.local/bin/codex",
	}
	config := loadConfig(func(name string) string { return values[name] })
	if config.HTTPAddr != "127.0.0.1:19081" || config.DataRoot != "/tmp/runner-data" {
		t.Fatalf("unexpected custom runner config: %#v", config)
	}
	if config.ClaudeBinary != "/home/runner/.local/bin/claude" || config.CodexBinary != "/home/runner/.local/bin/codex" {
		t.Fatalf("unexpected cli binary paths: %#v", config)
	}
}

func TestNewServiceRegistersCodeLevelCLIProviders(t *testing.T) {
	service, err := newService(config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("new runner service: %v", err)
	}
	if names := service.ProviderNames(); !reflect.DeepEqual(names, []string{"claude-cli", "codex-cli"}) {
		t.Fatalf("unexpected provider registry: %#v", names)
	}
}
