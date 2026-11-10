package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

type RecoveryHeadReader interface {
	ReadRecoveryHead(ctx context.Context, task *model.Task) (string, error)
}

type RecoveryPrerequisiteChecker interface {
	CheckRecoveryPrerequisite(ctx context.Context, info *failure.Info) (bool, error)
}

type RecoverySafetyReader struct {
	Store                      *repository.TaskRecoveryRepository
	RepositoryLookup           RepositoryLookup
	HeadReader                 RecoveryHeadReader
	PrerequisiteChecker        RecoveryPrerequisiteChecker
	ExecutionCapabilityChecker func(context.Context, *model.Task) error
	Now                        func() time.Time
}

func (r *RecoverySafetyReader) Read(ctx context.Context, taskID string) (RecoveryFacts, error) {
	if r == nil || r.Store == nil {
		return RecoveryFacts{}, fmt.Errorf("recovery snapshot store is required")
	}
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	snapshot, err := r.Store.LoadRecoverySnapshot(readCtx, taskID)
	if err != nil {
		return RecoveryFacts{}, err
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	facts := RecoveryFacts{
		Task:                snapshot.Task,
		Runs:                snapshot.Runs,
		Executions:          snapshot.Executions,
		ResultSubmissions:   snapshot.ResultSubmissions,
		Writebacks:          snapshot.Writebacks,
		Effects:             snapshot.Effects,
		ActionAdmissions:    snapshot.ActionAdmissions,
		WorkItemLeases:      snapshot.WorkItemLeases,
		DeliveryCheckpoints: snapshot.DeliveryCheckpoints,
		MergeGateAudit:      snapshot.MergeGateAudit,
		Stop:                snapshot.Stop,
		SnapshotVersion:     snapshot.Version,
		SafetyDigest:        repository.RecoverySafetyDigest(snapshot),
		CheckedAt:           now,
	}
	if snapshot.Task != nil {
		objectValid, objectReason, contextValid, contextReason, expectedHead := validateRecoveryTaskObject(snapshot.Task)
		facts.TaskObjectValid = &objectValid
		facts.TaskObjectReason = objectReason
		facts.ContextValid = &contextValid
		facts.ContextReason = contextReason
		facts.ExpectedHeadSHA = expectedHead
		facts.FailureInfo = recoveryFailureInfo(snapshot)
		facts.DispatchProvenNotSent = recoveryDispatchProvenNotSent(snapshot)
		if facts.FailureInfo != nil && failure.Classify(facts.FailureInfo) == failure.ClassificationPrerequisite && supportsPrerequisiteCheck(facts.FailureInfo.Code) && r.PrerequisiteChecker != nil {
			checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
			resolved, checkErr := r.PrerequisiteChecker.CheckRecoveryPrerequisite(checkCtx, facts.FailureInfo)
			checkCancel()
			if checkErr != nil {
				facts.PrerequisiteCheckError = checkErr.Error()
			} else {
				facts.PrerequisiteResolved = resolved
			}
		}
		if snapshot.Task.RepositoryID != nil && strings.TrimSpace(*snapshot.Task.RepositoryID) != "" {
			if r.RepositoryLookup == nil {
				facts.RepositoryLookupError = "repository lookup is unavailable"
			} else {
				lookupCtx, lookupCancel := context.WithTimeout(ctx, 5*time.Second)
				repo, lookupErr := r.RepositoryLookup.Get(lookupCtx, *snapshot.Task.RepositoryID)
				lookupCancel()
				if lookupErr != nil {
					facts.RepositoryLookupError = lookupErr.Error()
				} else {
					facts.Repository = repo
				}
			}
		} else {
			facts.RepositoryLookupError = "任务未绑定注册仓库"
		}
		role := strings.TrimSpace(snapshot.Task.Role)
		facts.HeadCheckRequired = (role == "ReviewerWorker" || role == "TesterWorker") && facts.ExpectedHeadSHA != ""
		if facts.HeadCheckRequired {
			if r.HeadReader == nil {
				facts.HeadCheckUnavailable = true
				facts.HeadCheckError = "PR/MR head reader is unavailable"
			} else {
				headCtx, headCancel := context.WithTimeout(ctx, 5*time.Second)
				head, headErr := r.HeadReader.ReadRecoveryHead(headCtx, snapshot.Task)
				headCancel()
				if headErr != nil {
					facts.HeadCheckUnavailable = true
					facts.HeadCheckError = headErr.Error()
				} else {
					facts.CurrentHeadSHA = head
				}
			}
		}
		if r.ExecutionCapabilityChecker != nil {
			checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
			if checkErr := r.ExecutionCapabilityChecker(checkCtx, snapshot.Task); checkErr != nil {
				facts.ExecutionCapabilityError = checkErr.Error()
			}
			checkCancel()
		}
	}
	return facts, nil
}

func recoveryFailureInfo(snapshot *repository.RecoverySnapshot) *failure.Info {
	if snapshot == nil || snapshot.Task == nil {
		return nil
	}
	source := sourceRun(RecoveryFacts{Task: snapshot.Task, Runs: snapshot.Runs})
	if source != nil {
		if execution := latestExecutionForRun(snapshot.Executions, source.RunID); execution != nil {
			return executionFailureInfo(execution)
		}
	}
	if strings.TrimSpace(snapshot.Task.LastError) != "" {
		return failure.FromLegacy("", snapshot.Task.LastError)
	}
	return nil
}

func recoveryDispatchProvenNotSent(snapshot *repository.RecoverySnapshot) bool {
	if snapshot == nil || snapshot.Task == nil {
		return false
	}
	source := sourceRun(RecoveryFacts{Task: snapshot.Task, Runs: snapshot.Runs})
	if source == nil {
		return false
	}
	for _, admission := range snapshot.ActionAdmissions {
		if admission.RunID == source.RunID && admission.Kind == model.ActionExecutionCreate && admission.NotSentAt != nil {
			return true
		}
	}
	return false
}

func validateRecoveryTaskObject(task *model.Task) (objectValid bool, objectReason string, contextValid bool, contextReason string, expectedHead string) {
	if task == nil {
		return false, "任务不存在", false, "任务上下文不存在", ""
	}
	contextValid = true
	context := map[string]any{}
	if strings.TrimSpace(task.ContextJSON) != "" {
		if err := json.Unmarshal([]byte(task.ContextJSON), &context); err != nil {
			contextValid = false
			contextReason = "任务上下文不是合法 JSON"
		}
	}
	objectValid = true
	switch strings.TrimSpace(task.Role) {
	case "ReviewerWorker", "TesterWorker":
		mergeRequest, ok := context["merge_request"].(map[string]any)
		if !ok {
			objectValid = false
			objectReason = "任务上下文缺少 merge_request"
			break
		}
		iid, iidOK := numericValue(mergeRequest["iid"])
		head, headOK := mergeRequest["head_sha"].(string)
		if !iidOK || iid <= 0 || !headOK || strings.TrimSpace(head) == "" {
			objectValid = false
			objectReason = "任务上下文的 PR/MR iid 或 head_sha 无效"
			break
		}
		expectedHead = strings.TrimSpace(head)
	case "DeveloperWorker":
		if strings.TrimSpace(task.TargetRepo) == "" && strings.TrimSpace(task.SourceRepo) == "" {
			objectValid = false
			objectReason = "任务缺少目标仓库"
		}
	}
	return objectValid, objectReason, contextValid, contextReason, expectedHead
}

func supportsPrerequisiteCheck(code string) bool {
	code = strings.TrimSpace(code)
	return code == failure.CodeCLIMissing || code == failure.CodeMissingConfiguration
}

func numericValue(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), typed == float64(int64(typed))
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
