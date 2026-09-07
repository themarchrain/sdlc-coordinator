package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

func BuildArtifactsFromResult(payload results.CanonicalResult) ([]model.Artifact, error) {
	if len(payload.Artifacts) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(payload.Artifacts))
	for key := range payload.Artifacts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	artifacts := make([]model.Artifact, 0, len(keys))
	for _, key := range keys {
		value := payload.Artifacts[key]
		metadata := map[string]any{
			"source_key": key,
			"value":      value,
		}
		rawMetadata, err := json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal artifact metadata %s: %w", key, err)
		}

		kind := artifactKindForKey(key)
		artifacts = append(artifacts, model.Artifact{
			TaskID:       payload.TaskID,
			RunID:        payload.RunID,
			Kind:         kind,
			Name:         key,
			URL:          artifactURLForValue(kind, key, value),
			MetadataJSON: string(rawMetadata),
		})
	}

	return artifacts, nil
}

func (o *Orchestrator) materializeResultArtifacts(ctx context.Context, payload results.CanonicalResult) error {
	if o.artifacts == nil || len(payload.Artifacts) == 0 {
		return nil
	}

	artifacts, err := BuildArtifactsFromResult(payload)
	if err != nil {
		return err
	}

	for i := range artifacts {
		if err := o.artifacts.UpsertByIdentity(ctx, &artifacts[i]); err != nil {
			return fmt.Errorf("upsert result artifact %s: %w", artifacts[i].Name, err)
		}
	}
	return nil
}

func artifactKindForKey(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "mr_url", "merge_request_url", "merge_request":
		return "merge_request"
	case "issue_url", "issue":
		return "issue"
	case "branch", "branch_name", "source_branch":
		return "branch"
	case "diff", "patch":
		return "diff"
	case "test_result", "test_results", "tests":
		return "test_result"
	case "commit", "commits":
		return "commit"
	default:
		return "generic"
	}
}

func artifactURLForValue(kind string, key string, value any) string {
	raw, ok := value.(string)
	if !ok {
		return ""
	}
	if kind == "merge_request" || kind == "issue" {
		return raw
	}
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if strings.HasSuffix(normalizedKey, "_url") && isHTTPURL(raw) {
		return raw
	}
	return ""
}

func isHTTPURL(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(normalized, "http://") || strings.HasPrefix(normalized, "https://")
}
