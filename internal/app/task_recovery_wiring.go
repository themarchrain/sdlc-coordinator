package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/registeredrepo"
)

type tenantRecoveryObjectReader struct {
	repositories *registeredrepo.Store
	resolver     *registeredrepo.CodeHostResolver
}

// ReadRecoveryObject reads the live state of the task's target object. A reader
// that cannot answer must return an error so recovery fails closed instead of
// treating an unreadable object as open.
func (r tenantRecoveryObjectReader) ReadRecoveryObject(ctx context.Context, task *model.Task) (orchestrator.RecoveryObjectState, error) {
	if task == nil || task.RepositoryID == nil || strings.TrimSpace(*task.RepositoryID) == "" {
		return orchestrator.RecoveryObjectState{}, fmt.Errorf("task repository is not bound")
	}
	repository, err := r.repositories.Get(ctx, *task.RepositoryID)
	if err != nil {
		return orchestrator.RecoveryObjectState{}, fmt.Errorf("load task repository: %w", err)
	}
	switch orchestrator.TaskRecoveryObjectKind(task) {
	case orchestrator.RecoveryObjectMergeRequest:
		reader, err := r.resolver.ResolveMergeRequestReader(ctx, *task.RepositoryID)
		if err != nil {
			return orchestrator.RecoveryObjectState{}, fmt.Errorf("resolve merge request reader: %w", err)
		}
		iid, err := taskMergeRequestIID(task)
		if err != nil {
			return orchestrator.RecoveryObjectState{}, err
		}
		mergeRequest, err := reader.GetMergeRequest(ctx, repository.PathWithNamespace, iid)
		if err != nil {
			return orchestrator.RecoveryObjectState{}, fmt.Errorf("read merge request state: %w", err)
		}
		return orchestrator.RecoveryObjectState{HeadSHA: strings.TrimSpace(mergeRequest.HeadSHA), State: mergeRequest.State}, nil
	case orchestrator.RecoveryObjectIssue:
		iid, ok := orchestrator.RecoveryIssueIID(task)
		if !ok {
			return orchestrator.RecoveryObjectState{}, fmt.Errorf("task context has no valid issue iid")
		}
		client, err := registeredrepo.CodeHostClient(repository.Provider, repository.BaseURL, repository.Token, nil)
		if err != nil {
			return orchestrator.RecoveryObjectState{}, err
		}
		state, err := client.GetIssueState(ctx, repository.PathWithNamespace, iid)
		if err != nil {
			return orchestrator.RecoveryObjectState{}, fmt.Errorf("read issue state: %w", err)
		}
		if state.Closed {
			return orchestrator.RecoveryObjectState{State: codehost.MergeRequestStateClosed}, nil
		}
		return orchestrator.RecoveryObjectState{State: codehost.MergeRequestStateOpen}, nil
	default:
		return orchestrator.RecoveryObjectState{}, fmt.Errorf("task has no live object to check")
	}
}

func taskMergeRequestIID(task *model.Task) (int, error) {
	if task == nil || strings.TrimSpace(task.ContextJSON) == "" {
		return 0, fmt.Errorf("task context has no merge_request")
	}
	var contextMap map[string]any
	if err := json.Unmarshal([]byte(task.ContextJSON), &contextMap); err != nil {
		return 0, fmt.Errorf("decode task context: %w", err)
	}
	mergeRequest, _ := contextMap["merge_request"].(map[string]any)
	switch value := mergeRequest["iid"].(type) {
	case float64:
		if value > 0 && value == float64(int(value)) {
			return int(value), nil
		}
	case json.Number:
		iid, err := value.Int64()
		if err == nil && iid > 0 {
			return int(iid), nil
		}
	}
	return 0, fmt.Errorf("task context has no valid merge_request.iid")
}
