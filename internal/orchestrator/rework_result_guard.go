package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

func validateReworkCanonicalResult(task *model.Task, canonical results.CanonicalResult) error {
	if task == nil || task.Type != model.TaskTypeReworkMergeRequest {
		return nil
	}
	if len(canonical.Writebacks) > 0 || len(canonical.NextTasks) > 0 || len(canonical.Effects) > 0 {
		return errors.New("rework_merge_request result must not include coordinator-owned intents: writebacks, next_tasks, or effects")
	}
	if canonical.Status != results.StatusSucceeded {
		return nil
	}
	if strings.TrimSpace(canonical.Summary) == "" {
		return errors.New("rework_merge_request succeeded result summary is required")
	}
	if artifactString(canonical.Artifacts, "commit_sha") == "" {
		return errors.New("rework_merge_request succeeded result commit_sha is required")
	}
	if artifactString(canonical.Artifacts, "diff") == "" {
		return errors.New("rework_merge_request succeeded result diff is required")
	}

	changedFiles, err := decodeReworkArtifact[[]string](canonical.Artifacts, "changed_files")
	if err != nil || len(changedFiles) == 0 {
		return errors.New("rework_merge_request succeeded result changed_files are required")
	}
	for index, file := range changedFiles {
		if strings.TrimSpace(file) == "" {
			return fmt.Errorf("rework_merge_request succeeded result changed_files[%d] is required", index)
		}
	}

	tests, err := decodeReworkArtifact[[]results.TestEvidence](canonical.Artifacts, "test_results")
	if err != nil || len(tests) == 0 {
		return errors.New("rework_merge_request succeeded result test_results are required")
	}
	for index, test := range tests {
		if strings.TrimSpace(test.Command) == "" || strings.TrimSpace(test.Summary) == "" {
			return fmt.Errorf("rework_merge_request succeeded result test_results[%d] command and summary are required", index)
		}
		if !test.Passed {
			return fmt.Errorf("rework_merge_request succeeded result test_results[%d] must pass", index)
		}
	}
	return nil
}

func decodeReworkArtifact[T any](artifacts map[string]any, key string) (T, error) {
	var value T
	if artifacts == nil || artifacts[key] == nil {
		return value, errors.New("artifact is missing")
	}
	raw, err := json.Marshal(artifacts[key])
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, err
	}
	return value, nil
}
