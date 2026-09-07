package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

func (o *Orchestrator) ResumeDeveloperTask(ctx context.Context, request model.DeveloperResumeRequest) (*model.Task, error) {
	if strings.TrimSpace(request.DedupeKey) == "" {
		return nil, errors.New("developer resume dedupe key is required")
	}
	if request.NoteID <= 0 {
		return nil, errors.New("developer resume note id is required")
	}
	if strings.TrimSpace(request.ResumeContext) == "" {
		return nil, errors.New("developer resume context is required")
	}

	task, err := o.tasks.GetByDedupeKey(ctx, request.DedupeKey)
	if err != nil {
		return nil, err
	}
	if task.Role != "DeveloperWorker" || task.Status != model.TaskStatusHumanRequired {
		return nil, repository.ErrTaskNotClaimable
	}

	contextJSON, err := resumedDeveloperTaskContext(task.ContextJSON, request)
	if err != nil {
		return nil, err
	}
	if err := o.tasks.ResumeHumanRequired(ctx, task.DedupeKey, contextJSON); err != nil {
		return nil, err
	}
	return o.tasks.GetByDedupeKey(ctx, task.DedupeKey)
}

func resumedDeveloperTaskContext(raw string, request model.DeveloperResumeRequest) (string, error) {
	context := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &context); err != nil {
		return "", fmt.Errorf("decode developer task context: %w", err)
	}
	issue, ok := context["issue"].(map[string]any)
	if !ok {
		return "", errors.New("developer task issue context is required")
	}

	issue["assignee_ids"] = append([]int(nil), request.AssigneeIDs...)
	issue["resume"] = map[string]any{
		"note_id": request.NoteID,
		"context": strings.TrimSpace(request.ResumeContext),
	}
	encoded, err := json.Marshal(context)
	if err != nil {
		return "", fmt.Errorf("encode resumed developer task context: %w", err)
	}
	return string(encoded), nil
}
