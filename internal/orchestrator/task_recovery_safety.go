package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

type RecoveryObjectKind string

const (
	RecoveryObjectNone         RecoveryObjectKind = ""
	RecoveryObjectMergeRequest RecoveryObjectKind = "merge_request"
	RecoveryObjectIssue        RecoveryObjectKind = "issue"
)

// RecoveryObjectKind reports which live object a task must still be able to act
// on. A missing or unknown object yields RecoveryObjectNone so the existing task
// object validation stays the gate for it.
func TaskRecoveryObjectKind(task *model.Task) RecoveryObjectKind {
	if task == nil {
		return RecoveryObjectNone
	}
	switch strings.TrimSpace(task.Role) {
	case "ReviewerWorker", "TesterWorker":
		return RecoveryObjectMergeRequest
	case "DeveloperWorker":
		if strings.TrimSpace(task.Type) == "rework_merge_request" {
			return RecoveryObjectMergeRequest
		}
		if _, ok := RecoveryIssueIID(task); ok {
			return RecoveryObjectIssue
		}
	}
	return RecoveryObjectNone
}

// RecoveryObjectState is the live state of the task's target object.
type RecoveryObjectState struct {
	HeadSHA string
	State   codehost.MergeRequestState
}

type RecoveryObjectReader interface {
	ReadRecoveryObject(ctx context.Context, task *model.Task) (RecoveryObjectState, error)
}

type RecoveryPrerequisiteChecker interface {
	CheckRecoveryPrerequisite(ctx context.Context, info *failure.Info) (bool, error)
}

type RecoverySafetyReader struct {
	Store               *repository.TaskRecoveryRepository
	RepositoryLookup    RepositoryLookup
	ObjectReader        RecoveryObjectReader
	PrerequisiteChecker RecoveryPrerequisiteChecker
	Now                 func() time.Time
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
		ExitConfirmations:   snapshot.ExitConfirmations,
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
		facts.DispatchProvenNotSentRuns = repository.RecoveryDispatchProvenNotSentRuns(snapshot)
		facts.DispatchProvenNotSent = facts.DispatchProvenNotSentRuns[sourceRunID(facts)]
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
		facts.ObjectStateCheckRequired = TaskRecoveryObjectKind(snapshot.Task) != RecoveryObjectNone
		r.readObjectState(ctx, snapshot.Task, &facts)
	}
	return facts, nil
}

// readObjectState fills the live head and object state used by the conflict
// gates. Any failure keeps the corresponding gate unconfirmed.
func (r *RecoverySafetyReader) readObjectState(ctx context.Context, task *model.Task, facts *RecoveryFacts) {
	if !facts.HeadCheckRequired && !facts.ObjectStateCheckRequired {
		return
	}
	unavailable := func(err error) {
		if facts.HeadCheckRequired {
			facts.HeadCheckUnavailable = true
			facts.HeadCheckError = err.Error()
		}
		if facts.ObjectStateCheckRequired {
			facts.ObjectStateUnavailable = true
			facts.ObjectStateError = err.Error()
		}
	}
	if r.ObjectReader == nil {
		unavailable(fmt.Errorf("task object reader is unavailable"))
		return
	}
	objectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	object, err := r.ObjectReader.ReadRecoveryObject(objectCtx, task)
	if err != nil {
		unavailable(err)
		return
	}
	facts.CurrentHeadSHA = strings.TrimSpace(object.HeadSHA)
	facts.CurrentObjectState = string(object.State)
	if facts.ObjectStateCheckRequired && !confirmableObjectState(facts.CurrentObjectState) {
		facts.ObjectStateUnavailable = true
		facts.ObjectStateError = "任务目标状态缺失或无法识别"
	}
}

// confirmableObjectState reports whether the platform actually answered: a
// missing or unrecognised word must not be read as open.
func confirmableObjectState(state string) bool {
	switch codehost.MergeRequestState(strings.ToLower(strings.TrimSpace(state))) {
	case codehost.MergeRequestStateOpen, codehost.MergeRequestStateClosed, codehost.MergeRequestStateMerged:
		return true
	}
	return false
}

func RecoveryIssueIID(task *model.Task) (int, bool) {
	if task == nil || strings.TrimSpace(task.ContextJSON) == "" {
		return 0, false
	}
	var context map[string]any
	if json.Unmarshal([]byte(task.ContextJSON), &context) != nil {
		return 0, false
	}
	issue, _ := context["issue"].(map[string]any)
	iid, ok := numericValue(issue["iid"])
	if !ok || iid <= 0 {
		return 0, false
	}
	return int(iid), true
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

// assertTaskObjectOpen refuses to resume a task whose live target object is no
// longer open. A reader that cannot answer fails closed: resuming on an
// unreadable object is the same bet as resuming on a closed one.
func (o *Orchestrator) assertTaskObjectOpen(ctx context.Context, task *model.Task) error {
	if TaskRecoveryObjectKind(task) == RecoveryObjectNone {
		return nil
	}
	if o.recoveryObjects == nil {
		return model.ErrTaskObjectUnavailable
	}
	state, err := o.recoveryObjects.ReadRecoveryObject(ctx, task)
	if err != nil {
		return fmt.Errorf("%w: %v", model.ErrTaskObjectUnavailable, err)
	}
	if state.State != codehost.MergeRequestStateOpen {
		return model.ErrTaskObjectNotOpen
	}
	return nil
}
