package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/operationmarker"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store/repository"
)

const (
	CodeHostWritebackCreateIssue            = "create_issue"
	CodeHostWritebackCreateMergeRequest     = "create_merge_request"
	CodeHostWritebackCreateIssueNote        = "create_issue_note"
	CodeHostWritebackCreateMergeRequestNote = "create_merge_request_note"
	CodeHostWritebackSetIssueLabels         = "set_issue_labels"
	CodeHostWritebackSetMergeRequestLabels  = "set_merge_request_labels"
	CodeHostWritebackAddMergeRequestLabels  = "add_merge_request_labels"
	GitLabWritebackCreateIssue              = CodeHostWritebackCreateIssue
	GitLabWritebackCreateMergeRequest       = CodeHostWritebackCreateMergeRequest
	GitLabWritebackCreateIssueNote          = CodeHostWritebackCreateIssueNote
	GitLabWritebackCreateMergeRequestNote   = CodeHostWritebackCreateMergeRequestNote
	GitLabWritebackSetIssueLabels           = CodeHostWritebackSetIssueLabels
	GitLabWritebackSetMergeRequestLabels    = CodeHostWritebackSetMergeRequestLabels
	GitLabWritebackAddMergeRequestLabels    = CodeHostWritebackAddMergeRequestLabels
)

// ErrWritebackRequiresReconciliation 表示写回远端结果未知(歧义),需等待对账
// 裁决。submission 重放层识别该哨兵后不计失败尝试——否则对账完成前重试预算
// 会被烧穿,把可自动恢复的瞬时故障放大为整任务重跑。
var ErrWritebackRequiresReconciliation = errors.New("codehost writeback requires reconciliation")

func BuildGitLabWritebacksFromResult(payload results.CanonicalResult) ([]model.Writeback, error) {
	return BuildCodeHostWritebacksFromResult(payload)
}

func BuildCodeHostWritebacksFromResult(payload results.CanonicalResult) ([]model.Writeback, error) {
	if len(payload.Writebacks) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(payload.TaskID) == "" {
		return nil, errors.New("result task_id is required for codehost writebacks")
	}
	if strings.TrimSpace(payload.RunID) == "" {
		return nil, errors.New("result run_id is required for codehost writebacks")
	}

	writebacks := make([]model.Writeback, 0, len(payload.Writebacks))
	for i, intent := range payload.Writebacks {
		if err := validateCodeHostWritebackIntent(intent); err != nil {
			return nil, fmt.Errorf("codehost writeback[%d]: %w", i, err)
		}

		intent.Kind = strings.TrimSpace(intent.Kind)
		intent.ProjectID = strings.TrimSpace(intent.ProjectID)
		intent.DedupeKey = strings.TrimSpace(intent.DedupeKey)
		if intent.DedupeKey == "" {
			intent.DedupeKey = fmt.Sprintf("codehost:%s:%s:%d:%s", payload.TaskID, payload.RunID, i, intent.Kind)
		}
		operationMarker := CodeHostOperationMarker(intent.DedupeKey)
		embedCodeHostOperationMarker(&intent, operationMarker)

		raw, err := json.Marshal(intent)
		if err != nil {
			return nil, fmt.Errorf("marshal codehost writeback[%d] request: %w", i, err)
		}

		writeback := model.Writeback{
			TaskID:          payload.TaskID,
			RunID:           payload.RunID,
			DedupeKey:       intent.DedupeKey,
			Kind:            intent.Kind,
			ProjectID:       intent.ProjectID,
			TargetType:      codeHostWritebackTargetType(intent),
			TargetIID:       codeHostWritebackTargetIID(intent),
			Status:          model.WritebackStatusPending,
			OperationMarker: operationMarker,
			RequestJSON:     string(raw),
		}
		writebacks = append(writebacks, writeback)
	}

	return writebacks, nil
}

func CodeHostOperationMarker(dedupeKey string) string {
	return operationmarker.New(dedupeKey)
}

func embedCodeHostOperationMarker(intent *results.CodeHostWritebackIntent, marker string) {
	if intent == nil || strings.TrimSpace(marker) == "" {
		return
	}
	switch strings.TrimSpace(intent.Kind) {
	case CodeHostWritebackCreateIssue, CodeHostWritebackCreateMergeRequest:
		intent.Description = appendOperationMarker(intent.Description, marker)
	case CodeHostWritebackCreateIssueNote, CodeHostWritebackCreateMergeRequestNote:
		intent.Body = appendOperationMarker(intent.Body, marker)
	}
}

func appendOperationMarker(body, marker string) string {
	if strings.Contains(body, marker) {
		return body
	}
	if strings.TrimSpace(body) == "" {
		return marker
	}
	return strings.TrimRight(body, "\r\n") + "\n\n" + marker
}

func validateCodeHostWritebackIntent(intent results.CodeHostWritebackIntent) error {
	kind := strings.TrimSpace(intent.Kind)
	if kind == "" {
		return errors.New("kind is required")
	}
	if strings.TrimSpace(intent.ProjectID) == "" {
		return errors.New("project_id is required")
	}

	switch kind {
	case CodeHostWritebackCreateIssue:
		if strings.TrimSpace(intent.Title) == "" {
			return errors.New("title is required for create_issue")
		}
	case CodeHostWritebackCreateMergeRequest:
		if strings.TrimSpace(intent.Title) == "" {
			return errors.New("title is required for create_merge_request")
		}
		if strings.TrimSpace(intent.SourceBranch) == "" {
			return errors.New("source_branch is required for create_merge_request")
		}
		if strings.TrimSpace(intent.TargetBranch) == "" {
			return errors.New("target_branch is required for create_merge_request")
		}
	case CodeHostWritebackCreateIssueNote:
		if intent.IssueIID <= 0 {
			return errors.New("issue_iid is required for create_issue_note")
		}
		if strings.TrimSpace(intent.Body) == "" {
			return errors.New("body is required for create_issue_note")
		}
	case CodeHostWritebackCreateMergeRequestNote:
		if intent.MergeRequestIID <= 0 {
			return errors.New("merge_request_iid is required for create_merge_request_note")
		}
		if strings.TrimSpace(intent.Body) == "" {
			return errors.New("body is required for create_merge_request_note")
		}
	case CodeHostWritebackSetIssueLabels:
		if intent.IssueIID <= 0 {
			return errors.New("issue_iid is required for set_issue_labels")
		}
		if len(intent.Labels) == 0 {
			return errors.New("labels are required for set_issue_labels")
		}
	case CodeHostWritebackSetMergeRequestLabels, CodeHostWritebackAddMergeRequestLabels:
		if intent.MergeRequestIID <= 0 {
			return errors.New("merge_request_iid is required for set_merge_request_labels")
		}
		if len(intent.Labels) == 0 {
			return errors.New("labels are required for set_merge_request_labels")
		}
	default:
		return fmt.Errorf("unsupported kind %q", kind)
	}

	return nil
}

func validateCodeHostWritebacksForTask(task *model.Task, payload results.CanonicalResult) error {
	if task == nil {
		return errors.New("task is required for codehost writeback validation")
	}

	expectedBranch := model.DeliveryBranch(task)
	expectedProject := strings.TrimSpace(task.TargetRepo)
	for i, intent := range payload.Writebacks {
		if strings.TrimSpace(intent.Kind) != CodeHostWritebackCreateMergeRequest {
			continue
		}
		actualProject := strings.TrimSpace(intent.ProjectID)
		if expectedProject == "" || actualProject != expectedProject {
			return fmt.Errorf(
				"codehost writeback[%d]: project_id %q does not match delivery repository %q",
				i,
				actualProject,
				expectedProject,
			)
		}
		actualBranch := strings.TrimSpace(intent.SourceBranch)
		if actualBranch != expectedBranch {
			return fmt.Errorf(
				"codehost writeback[%d]: source_branch %q does not match delivery branch %q",
				i,
				actualBranch,
				expectedBranch,
			)
		}
	}
	return nil
}

func codeHostWritebackTargetType(intent results.CodeHostWritebackIntent) string {
	switch strings.TrimSpace(intent.Kind) {
	case CodeHostWritebackCreateIssueNote, CodeHostWritebackSetIssueLabels:
		return "issue"
	case CodeHostWritebackCreateMergeRequestNote, CodeHostWritebackSetMergeRequestLabels, CodeHostWritebackAddMergeRequestLabels:
		return "merge_request"
	default:
		return ""
	}
}

func codeHostWritebackTargetIID(intent results.CodeHostWritebackIntent) int {
	switch strings.TrimSpace(intent.Kind) {
	case CodeHostWritebackCreateIssueNote, CodeHostWritebackSetIssueLabels:
		return intent.IssueIID
	case CodeHostWritebackCreateMergeRequestNote, CodeHostWritebackSetMergeRequestLabels, CodeHostWritebackAddMergeRequestLabels:
		return intent.MergeRequestIID
	default:
		return 0
	}
}

func decodeCodeHostWritebackIntent(writeback *model.Writeback) (results.CodeHostWritebackIntent, error) {
	if writeback == nil {
		return results.CodeHostWritebackIntent{}, errors.New("writeback is required")
	}
	var intent results.CodeHostWritebackIntent
	if err := json.Unmarshal([]byte(writeback.RequestJSON), &intent); err != nil {
		return results.CodeHostWritebackIntent{}, fmt.Errorf("decode codehost writeback request: %w", err)
	}
	if intent.DedupeKey == "" {
		intent.DedupeKey = writeback.DedupeKey
	}
	if intent.Kind == "" {
		intent.Kind = writeback.Kind
	}
	if intent.ProjectID == "" {
		intent.ProjectID = writeback.ProjectID
	}
	return intent, nil
}

func writebackExternalIDFromInt(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func (o *Orchestrator) applyCodeHostWritebacks(ctx context.Context, payload results.CanonicalResult) error {
	if len(payload.Writebacks) == 0 {
		return nil
	}
	if o.writebacks == nil {
		return errors.New("writeback repository is required")
	}
	if o.codeHostResolver == nil {
		return errors.New("codehost writeback resolver is required")
	}

	writebacks, err := BuildCodeHostWritebacksFromResult(payload)
	if err != nil {
		return err
	}

	for i := range writebacks {
		if err := o.applyOneCodeHostWriteback(ctx, &writebacks[i]); err != nil {
			return err
		}
	}
	return nil
}

func (o *Orchestrator) applyOneCodeHostWriteback(ctx context.Context, desired *model.Writeback) error {
	writeback := desired
	existing, err := o.writebacks.GetByDedupeKey(ctx, desired.DedupeKey)
	if err == nil {
		switch existing.Status {
		case model.WritebackStatusSucceeded:
			return o.materializeCodeHostWritebackArtifact(ctx, existing)
		case model.WritebackStatusFailed:
			return fmt.Errorf("codehost writeback %s previously failed: %s", existing.DedupeKey, existing.ErrorMessage)
		case model.WritebackStatusUnknown:
			return fmt.Errorf("codehost writeback %s has unknown remote outcome and requires reconciliation: %s: %w", existing.DedupeKey, existing.ErrorMessage, ErrWritebackRequiresReconciliation)
		case model.WritebackStatusRunning:
			// A persisted running state can only be replayed after the previous
			// caller lost its terminal database update (for example, the process
			// crashed after the remote mutation succeeded). Re-executing here could
			// create a duplicate MR/comment, so force the marker-based reconciliation
			// path before any further remote mutation.
			existing.Status = model.WritebackStatusUnknown
			existing.ErrorMessage = "previous writeback attempt ended without a persisted remote outcome"
			if err := o.writebacks.Update(ctx, existing); err != nil {
				return err
			}
			return fmt.Errorf("codehost writeback %s has an ambiguous previous running attempt and requires reconciliation: %w", existing.DedupeKey, ErrWritebackRequiresReconciliation)
		default:
			existing.TaskID = desired.TaskID
			existing.RunID = desired.RunID
			existing.Kind = desired.Kind
			existing.ProjectID = desired.ProjectID
			existing.TargetType = desired.TargetType
			existing.TargetIID = desired.TargetIID
			existing.RequestJSON = desired.RequestJSON
			writeback = existing
		}
	} else if errors.Is(err, repository.ErrNotFound) {
		if err := o.writebacks.UpsertByDedupeKey(ctx, writeback); err != nil {
			return err
		}
	} else {
		return err
	}

	writeback.Status = model.WritebackStatusRunning
	writeback.AttemptCount++
	writeback.ErrorMessage = ""
	if err := o.writebacks.Update(ctx, writeback); err != nil {
		return err
	}

	result, err := o.executeCodeHostWriteback(ctx, writeback)
	if err != nil {
		writeback.Status = model.WritebackStatusFailed
		if codehost.IsAmbiguousError(err) {
			writeback.Status = model.WritebackStatusUnknown
		} else if codehost.IsTransientRejection(err) {
			// 服务端明确拒绝且确定未应用(HTTP 429/408):保持 Pending 可重执行,
			// submission 重放会真正重试远端调用,而不是被 Failed 态短路。
			writeback.Status = model.WritebackStatusPending
		}
		writeback.ErrorMessage = err.Error()
		_ = o.writebacks.Update(ctx, writeback)
		if parkErr := o.parkTaskIfWritebackRequiresHuman(ctx, writeback.TaskID, err); parkErr != nil {
			return parkErr
		}
		return fmt.Errorf("execute codehost writeback %s: %w", writeback.DedupeKey, err)
	}

	writeback.Status = model.WritebackStatusSucceeded
	writeback.ResponseJSON = result.responseJSON
	writeback.ExternalID = result.externalID
	writeback.ExternalURL = result.externalURL
	if err := o.writebacks.Update(ctx, writeback); err != nil {
		return err
	}

	return o.materializeCodeHostWritebackArtifact(ctx, writeback)
}

type codeHostWritebackResult struct {
	externalID   string
	externalURL  string
	responseJSON string
}

func (o *Orchestrator) executeCodeHostWriteback(ctx context.Context, writeback *model.Writeback) (codeHostWritebackResult, error) {
	intent, err := decodeCodeHostWritebackIntent(writeback)
	if err != nil {
		return codeHostWritebackResult{}, err
	}
	backend, err := o.resolveCodeHostBackend(ctx, writeback)
	if err != nil {
		return codeHostWritebackResult{}, err
	}

	switch writeback.Kind {
	case CodeHostWritebackCreateIssue:
		issue, err := backend.CreateIssue(ctx, writeback.ProjectID, codehost.CreateIssueRequest{
			Title:       intent.Title,
			Description: intent.Description,
			Labels:      intent.Labels,
		})
		if err != nil {
			return codeHostWritebackResult{}, err
		}
		return codeHostWritebackResultFromResponse(issue.IID, issue.WebURL, issue)
	case CodeHostWritebackCreateMergeRequest:
		mr, err := backend.CreateMergeRequest(ctx, writeback.ProjectID, codehost.CreateMergeRequestRequest{
			Title:              intent.Title,
			Description:        intent.Description,
			SourceBranch:       intent.SourceBranch,
			TargetBranch:       intent.TargetBranch,
			RemoveSourceBranch: intent.RemoveSourceBranch,
			Labels:             intent.Labels,
		})
		if err != nil {
			return codeHostWritebackResult{}, err
		}
		return codeHostWritebackResultFromResponse(mr.IID, mr.WebURL, mr)
	case CodeHostWritebackCreateIssueNote:
		note, err := backend.CreateIssueNote(ctx, writeback.ProjectID, intent.IssueIID, codehost.CreateNoteRequest{Body: intent.Body})
		if err != nil {
			return codeHostWritebackResult{}, err
		}
		return codeHostWritebackResultFromResponse(note.ID, "", note)
	case CodeHostWritebackCreateMergeRequestNote:
		note, err := backend.CreateMergeRequestNote(ctx, writeback.ProjectID, intent.MergeRequestIID, codehost.CreateNoteRequest{Body: intent.Body})
		if err != nil {
			return codeHostWritebackResult{}, err
		}
		return codeHostWritebackResultFromResponse(note.ID, "", note)
	case CodeHostWritebackSetIssueLabels:
		issue, err := backend.SetIssueLabels(ctx, writeback.ProjectID, intent.IssueIID, codehost.SetLabelsRequest{Labels: intent.Labels})
		if err != nil {
			return codeHostWritebackResult{}, err
		}
		return codeHostWritebackResultFromResponse(issue.IID, issue.WebURL, issue)
	case CodeHostWritebackSetMergeRequestLabels:
		mr, err := backend.SetMergeRequestLabels(ctx, writeback.ProjectID, intent.MergeRequestIID, codehost.SetLabelsRequest{Labels: intent.Labels})
		if err != nil {
			return codeHostWritebackResult{}, err
		}
		return codeHostWritebackResultFromResponse(mr.IID, mr.WebURL, mr)
	case CodeHostWritebackAddMergeRequestLabels:
		adder, ok := backend.(codehost.MergeRequestLabelAdder)
		if !ok {
			return codeHostWritebackResult{}, errors.New("codehost backend does not support additive merge request labels")
		}
		mr, err := adder.AddMergeRequestLabels(ctx, writeback.ProjectID, intent.MergeRequestIID, codehost.SetLabelsRequest{Labels: intent.Labels})
		if err != nil {
			return codeHostWritebackResult{}, err
		}
		return codeHostWritebackResultFromResponse(mr.IID, mr.WebURL, mr)
	default:
		return codeHostWritebackResult{}, fmt.Errorf("unsupported codehost writeback kind %q", writeback.Kind)
	}
}

// resolveCodeHostBackend 经 per-repo resolver 解析写回后端。任务必须绑定注册仓，
// 凭据取该仓自身的 Token。
func (o *Orchestrator) resolveCodeHostBackend(ctx context.Context, writeback *model.Writeback) (codehost.Backend, error) {
	if o.codeHostResolver == nil {
		return nil, errors.New("codehost writeback resolver is required")
	}
	task, err := o.tasks.GetByTaskID(ctx, writeback.TaskID)
	if err != nil {
		return nil, err
	}
	if task.RepositoryID == nil || strings.TrimSpace(*task.RepositoryID) == "" {
		return nil, ErrRepositoryNotBound
	}
	backend, err := o.codeHostResolver.Resolve(ctx, task, writeback)
	if err != nil {
		return nil, mapCodeHostResolveError(err)
	}
	return backend, nil
}

func codeHostWritebackResultFromResponse(externalID int, externalURL string, response any) (codeHostWritebackResult, error) {
	raw, err := json.Marshal(response)
	if err != nil {
		return codeHostWritebackResult{}, fmt.Errorf("marshal codehost writeback response: %w", err)
	}
	return codeHostWritebackResult{
		externalID:   writebackExternalIDFromInt(externalID),
		externalURL:  externalURL,
		responseJSON: string(raw),
	}, nil
}

func (o *Orchestrator) materializeCodeHostWritebackArtifact(ctx context.Context, writeback *model.Writeback) error {
	if o.artifacts == nil || writeback == nil {
		return nil
	}

	rawMetadata, err := json.Marshal(map[string]any{
		"source":       "codehost_writeback",
		"writeback_id": writeback.WritebackID,
		"dedupe_key":   writeback.DedupeKey,
		"kind":         writeback.Kind,
		"project_id":   writeback.ProjectID,
		"external_id":  writeback.ExternalID,
		"response":     json.RawMessage(nonEmptyJSON(writeback.ResponseJSON)),
	})
	if err != nil {
		return fmt.Errorf("marshal codehost writeback artifact metadata: %w", err)
	}

	artifact := &model.Artifact{
		TaskID:       writeback.TaskID,
		RunID:        writeback.RunID,
		Kind:         artifactKindForCodeHostWriteback(writeback.Kind),
		Name:         "writeback:" + writeback.DedupeKey,
		URL:          writeback.ExternalURL,
		MetadataJSON: string(rawMetadata),
	}
	if err := o.artifacts.UpsertByIdentity(ctx, artifact); err != nil {
		return fmt.Errorf("upsert codehost writeback artifact %s: %w", writeback.DedupeKey, err)
	}
	return nil
}

func artifactKindForCodeHostWriteback(kind string) string {
	switch kind {
	case CodeHostWritebackCreateIssue:
		return "issue"
	case CodeHostWritebackCreateMergeRequest:
		return "merge_request"
	case CodeHostWritebackCreateIssueNote, CodeHostWritebackCreateMergeRequestNote:
		return "comment"
	case CodeHostWritebackSetIssueLabels, CodeHostWritebackSetMergeRequestLabels, CodeHostWritebackAddMergeRequestLabels:
		return "label"
	default:
		return "generic"
	}
}

func nonEmptyJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

func mapCodeHostResolveError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrRepositoryNotEnabled) || err.Error() == ErrRepositoryNotEnabled.Error() {
		return ErrRepositoryNotEnabled
	}
	if errors.Is(err, ErrRepositoryNotBound) || err.Error() == ErrRepositoryNotBound.Error() {
		return ErrRepositoryNotBound
	}
	return err
}

func writebackRequiresHuman(err error) bool {
	return errors.Is(err, ErrRepositoryNotEnabled) || errors.Is(err, ErrRepositoryNotBound)
}

func (o *Orchestrator) parkTaskIfWritebackRequiresHuman(ctx context.Context, taskID string, cause error) error {
	task, err := o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	park := writebackRequiresHuman(cause)
	if o.writebackAuth != nil && o.writebackAuth.HandleWritebackFailure(ctx, task, cause) {
		park = true
	}
	if !park {
		return nil
	}
	previous := task.Status
	if err := o.stateMachine.Transition(task, model.TaskStatusHumanRequired); err != nil {
		return err
	}
	o.policy.Apply(task, previous, model.TaskStatusHumanRequired, o.now())
	task.LastError = cause.Error()
	if repo := o.lookupRegisteredRepo(ctx, task); repo != nil {
		token := strings.TrimSpace(repo.Token)
		if token != "" {
			task.LastError = strings.ReplaceAll(task.LastError, token, "***")
		}
	}
	if err := o.tasks.Update(ctx, task); err != nil {
		return fmt.Errorf("park writeback task %s: %w", taskID, err)
	}
	return nil
}
