package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/runner"
)

type tenantRecoveryHeadReader struct {
	repositories *registeredrepo.Store
	resolver     *registeredrepo.CodeHostResolver
}

func (r tenantRecoveryHeadReader) ReadRecoveryHead(ctx context.Context, task *model.Task) (string, error) {
	if task == nil || task.RepositoryID == nil || strings.TrimSpace(*task.RepositoryID) == "" {
		return "", fmt.Errorf("task repository is not bound")
	}
	repository, err := r.repositories.Get(ctx, *task.RepositoryID)
	if err != nil {
		return "", fmt.Errorf("load task repository: %w", err)
	}
	reader, err := r.resolver.ResolveMergeRequestReader(ctx, *task.RepositoryID)
	if err != nil {
		return "", fmt.Errorf("resolve merge request reader: %w", err)
	}
	iid, err := taskMergeRequestIID(task)
	if err != nil {
		return "", err
	}
	mergeRequest, err := reader.GetMergeRequest(ctx, repository.PathWithNamespace, iid)
	if err != nil {
		return "", fmt.Errorf("read merge request head: %w", err)
	}
	return strings.TrimSpace(mergeRequest.HeadSHA), nil
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

type tenantRecoveryPrerequisiteChecker struct {
	provider runner.ProviderConfig
}

func (c tenantRecoveryPrerequisiteChecker) CheckRecoveryPrerequisite(_ context.Context, info *failure.Info) (bool, error) {
	if info == nil {
		return false, nil
	}
	switch strings.TrimSpace(info.Code) {
	case failure.CodeCLIMissing, failure.CodeMissingConfiguration:
		return strings.TrimSpace(c.provider.Name) != "", nil
	default:
		return false, nil
	}
}
