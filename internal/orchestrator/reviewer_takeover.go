package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

const reviewerTakeoverLabel = "reviewer-agent"

func (o *Orchestrator) markReviewerTakenOver(ctx context.Context, task *model.Task, run *model.Run) error {
	if task == nil || run == nil || task.Role != "ReviewerWorker" {
		return nil
	}
	projectID, mrIID, err := reviewerTakeoverTarget(task)
	if err != nil {
		return err
	}
	return o.applyCodeHostWritebacks(ctx, results.CanonicalResult{
		TaskID: task.TaskID,
		RunID:  run.RunID,
		Writebacks: []results.CodeHostWritebackIntent{{
			DedupeKey:       "reviewer-takeover-label:" + task.TaskID,
			Kind:            CodeHostWritebackAddMergeRequestLabels,
			ProjectID:       projectID,
			MergeRequestIID: mrIID,
			Labels:          []string{reviewerTakeoverLabel},
		}},
	})
}

func reviewerTakeoverTarget(task *model.Task) (string, int, error) {
	var taskContext struct {
		Project struct {
			Path string `json:"path"`
		} `json:"project"`
		MergeRequest struct {
			IID int `json:"iid"`
		} `json:"merge_request"`
	}
	if err := json.Unmarshal([]byte(task.ContextJSON), &taskContext); err != nil {
		return "", 0, fmt.Errorf("decode reviewer takeover context: %w", err)
	}
	projectID := strings.TrimSpace(taskContext.Project.Path)
	if projectID == "" {
		projectID = strings.TrimSpace(task.TargetRepo)
	}
	if projectID == "" || taskContext.MergeRequest.IID <= 0 {
		return "", 0, errors.New("reviewer takeover project and merge request iid are required")
	}
	return projectID, taskContext.MergeRequest.IID, nil
}
