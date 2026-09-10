package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"sdlc-coordinator/internal/tools/gitlabsmoke"
)

func main() {
	var opts gitlabsmoke.Options

	flag.StringVar(&opts.BaseURL, "base-url", env("COORDINATOR_BASE_URL", "http://localhost:8080"), "coordinator base URL")
	flag.StringVar(&opts.PayloadPath, "payload", env("GITLAB_FIXTURE_PATH", ""), "GitLab webhook JSON payload file")
	flag.StringVar(&opts.Secret, "secret", env("GITLAB_WEBHOOK_SECRET", ""), "GitLab webhook shared secret sent as X-Gitlab-Token")
	flag.StringVar(&opts.EventName, "event", env("GITLAB_EVENT_NAME", "Merge Request Hook"), "GitLab event header, for example: Merge Request Hook")
	flag.StringVar(&opts.EventUUID, "event-uuid", env("GITLAB_EVENT_UUID", ""), "GitLab event UUID header; generated when empty")
	flag.StringVar(&opts.WebhookUUID, "webhook-uuid", env("GITLAB_WEBHOOK_UUID", ""), "GitLab webhook delivery UUID header")
	flag.StringVar(&opts.InstanceURL, "instance", env("GITLAB_INSTANCE_URL", ""), "GitLab instance URL header")
	flag.BoolVar(&opts.DryRun, "dry-run", false, "print replay requests without sending them")
	flag.Parse()

	opts.Output = os.Stdout

	result, err := gitlabsmoke.Run(context.Background(), opts)
	if result != nil && !opts.DryRun {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		_ = encoder.Encode(result)
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
