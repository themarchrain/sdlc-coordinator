package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store/repository"
)

const (
	RecoveryReasonEligible             = "eligible"
	RecoveryReasonUnknownConfirmation  = "unknown_risk_confirmation_required"
	RecoveryReasonFailurePrerequisite  = "failure_prerequisite"
	RecoveryReasonFailureFatal         = "failure_fatal"
	RecoveryReasonPrerequisiteResolved = "prerequisite_resolved"
)

const (
	RecoverySafetyTaskNotFound              = "task_not_found"
	RecoverySafetyTaskNotBlocked            = "task_not_blocked"
	RecoverySafetyTenantInvalid             = "tenant_invalid"
	RecoverySafetyPermissionDenied          = "permission_denied"
	RecoverySafetyStopRequested             = "stop_requested"
	RecoverySafetyExecutionActive           = "execution_active"
	RecoverySafetyExecutionUnconfirmed      = "execution_unconfirmed"
	RecoverySafetySubmissionProcessing      = "submission_processing"
	RecoverySafetySubmissionUnsettled       = "submission_unsettled"
	RecoverySafetyWritebackPending          = "writeback_pending"
	RecoverySafetyWritebackUnknown          = "writeback_unknown"
	RecoverySafetyActionUnsettled           = "action_unsettled"
	RecoverySafetyCriticalEffectPending     = "critical_effect_pending"
	RecoverySafetyActiveExecutionLease      = "active_execution_lease"
	RecoverySafetyMergeAuditUnsettled       = "merge_audit_unsettled"
	RecoverySafetyRepositoryNotBound        = "repository_not_bound"
	RecoverySafetyRepositoryLookupFailed    = "repository_lookup_failed"
	RecoverySafetyRepositoryDisabled        = "repository_disabled"
	RecoverySafetyTaskObjectInvalid         = "task_object_invalid"
	RecoverySafetyTaskContextInvalid        = "task_context_invalid"
	RecoverySafetyExecutionCapabilityFailed = "execution_capability_unavailable"
	RecoverySafetyPrerequisiteCheckFailed   = "prerequisite_check_unavailable"
	RecoverySafetyHeadCheckUnavailable      = "head_check_unavailable"
	RecoverySafetyHeadSuperseded            = "head_superseded"
	RecoverySafetySuccessorConflict         = "successor_conflict"
	RecoverySafetyCheckUnavailable          = "safety_check_unavailable"
)

type SafetyBlocker struct {
	Code        string `json:"code"`
	Summary     string `json:"summary"`
	Remediation string `json:"remediation,omitempty"`
}

type RecoveryFacts struct {
	Task                     *model.Task
	Runs                     []model.Run
	Executions               []model.AgentExecution
	ResultSubmissions        []model.ResultSubmission
	Writebacks               []model.Writeback
	Effects                  []model.Effect
	ActionAdmissions         []model.ActionAdmission
	WorkItemLeases           []model.WorkItemLease
	DeliveryCheckpoints      []model.DeveloperDeliveryCheckpoint
	MergeGateAudit           *model.MergeGateAudit
	Stop                     *model.StopRequest
	Repository               *registeredrepo.RegisteredRepository
	RepositoryLookupError    string
	TaskObjectValid          *bool
	TaskObjectReason         string
	ContextValid             *bool
	ContextReason            string
	ExecutionCapabilityError string
	PrerequisiteResolved     bool
	PrerequisiteCheckError   string
	ExpectedHeadSHA          string
	CurrentHeadSHA           string
	HeadCheckRequired        bool
	HeadCheckUnavailable     bool
	HeadCheckError           string
	SuccessorConflict        bool
	SuccessorTaskID          string
	DispatchProvenNotSent    bool
	ExpectedRequestHash      string
	AccessDenied             bool
	TenantInvalid            bool
	SafetyBlockers           []SafetyBlocker
	SnapshotVersion          string
	SafetyDigest             string
	CheckedAt                time.Time
	FailureInfo              *failure.Info
}

type RecoveryAssessment struct {
	Classification           failure.Classification `json:"classification"`
	Eligible                 bool                   `json:"eligible"`
	RequiresRiskConfirmation bool                   `json:"requires_risk_confirmation"`
	ReasonCode               string                 `json:"reason_code"`
	ReasonSummary            string                 `json:"reason_summary"`
	Remediation              string                 `json:"remediation,omitempty"`
	SourceRunID              string                 `json:"source_run_id,omitempty"`
	CheckedAt                time.Time              `json:"checked_at"`
	BlockRevision            string                 `json:"block_revision"`
	SafetyBlockers           []SafetyBlocker        `json:"safety_blockers"`
	FailureStage             string                 `json:"failure_stage,omitempty"`
	FailureCode              string                 `json:"failure_code,omitempty"`
	FailureSummary           string                 `json:"failure_summary,omitempty"`
	DBSnapshotVersion        string                 `json:"db_snapshot_version,omitempty"`
	TaskVersion              string                 `json:"-"`
	SafetyDigest             string                 `json:"-"`
	RecoveryGeneration       uint64                 `json:"-"`
}

type RecoveryFactsReader interface {
	Read(ctx context.Context, taskID string) (RecoveryFacts, error)
}

func (o *Orchestrator) AssessTaskRecovery(ctx context.Context, taskID string) (RecoveryAssessment, error) {
	return AssessTaskRecovery(ctx, o.recoveryReader, taskID)
}

func AssessTaskRecovery(ctx context.Context, reader RecoveryFactsReader, taskID string) (RecoveryAssessment, error) {
	if reader == nil {
		return RecoveryAssessment{Classification: failure.ClassificationUnknown, ReasonCode: RecoverySafetyCheckUnavailable, ReasonSummary: "恢复安全检查不可用"}, fmt.Errorf("recovery facts reader is required")
	}
	assessmentCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	facts, err := reader.Read(assessmentCtx, taskID)
	if err != nil {
		return RecoveryAssessment{
			Classification: failure.Classify(nil),
			ReasonCode:     RecoverySafetyCheckUnavailable,
			ReasonSummary:  "恢复安全检查暂不可用，请稍后重试",
			CheckedAt:      time.Now().UTC(),
		}, fmt.Errorf("assess task recovery: %w", err)
	}
	return EvaluateTaskRecovery(facts), nil
}

func EvaluateTaskRecovery(facts RecoveryFacts) RecoveryAssessment {
	info := facts.FailureInfo
	assessment := RecoveryAssessment{
		Classification:    failure.Classify(info),
		CheckedAt:         facts.CheckedAt,
		SourceRunID:       sourceRunID(facts),
		DBSnapshotVersion: facts.SnapshotVersion,
		SafetyDigest:      facts.SafetyDigest,
	}
	if facts.Task != nil {
		assessment.TaskVersion = repository.RecoveryTaskVersion(facts.Task)
		assessment.RecoveryGeneration = facts.Task.RecoveryGeneration
	}
	if assessment.CheckedAt.IsZero() {
		assessment.CheckedAt = time.Now().UTC()
	}
	if info != nil {
		assessment.FailureStage = info.Stage
		assessment.FailureCode = info.Code
		assessment.FailureSummary = info.Summary
	}
	if facts.Task != nil {
		assessment.BlockRevision = repository.RecoveryBlockRevision(facts.Task, facts.SafetyDigest)
	}
	blockers := append([]SafetyBlocker(nil), facts.SafetyBlockers...)
	addBlocker := func(code, summary, remediation string) {
		for _, existing := range blockers {
			if existing.Code == code {
				return
			}
		}
		blockers = append(blockers, SafetyBlocker{Code: code, Summary: summary, Remediation: remediation})
	}

	if facts.TenantInvalid {
		addBlocker(RecoverySafetyTenantInvalid, "任务所属组织无效", "请使用有效组织重新登录后重试")
	}
	if facts.AccessDenied {
		addBlocker(RecoverySafetyPermissionDenied, "当前用户没有本组织恢复权限", "请联系组织管理员")
	}
	if facts.Task == nil {
		addBlocker(RecoverySafetyTaskNotFound, "任务不存在", "")
	} else {
		if facts.Task.Status != model.TaskStatusBlocked {
			addBlocker(RecoverySafetyTaskNotBlocked, fmt.Sprintf("任务当前状态为 %s，只有 blocked 任务可以恢复", facts.Task.Status), "请刷新任务状态")
		}
		if facts.Stop != nil {
			addBlocker(RecoverySafetyStopRequested, "任务存在人工停止决定，停止优先于恢复", "")
		}
	}

	evaluateExecutionSafety(facts, addBlocker)
	evaluateExternalActions(facts, addBlocker)
	evaluateObjectAndRepository(facts, addBlocker)
	evaluateConflicts(facts, addBlocker)

	assessment.SafetyBlockers = blockers
	if len(blockers) > 0 {
		assessment.Eligible = false
		assessment.RequiresRiskConfirmation = false
		assessment.ReasonCode = blockers[0].Code
		assessment.ReasonSummary = blockers[0].Summary
		assessment.Remediation = blockers[0].Remediation
		return assessment
	}

	switch assessment.Classification {
	case failure.ClassificationPrerequisite:
		if facts.PrerequisiteResolved {
			assessment.Eligible = true
			assessment.ReasonCode = RecoveryReasonPrerequisiteResolved
			assessment.ReasonSummary = "前置条件已修复，恢复后将按正常策略重新排队"
			break
		}
		assessment.Eligible = false
		assessment.ReasonCode = RecoveryReasonFailurePrerequisite
		assessment.ReasonSummary = firstNonEmpty(infoSummary(info), "任务前置条件未满足")
		assessment.Remediation = firstNonEmpty(infoSummary(info), "请先修复错误摘要中的配置或权限后重新评估")
	case failure.ClassificationFatal:
		assessment.Eligible = false
		assessment.ReasonCode = RecoveryReasonFailureFatal
		assessment.ReasonSummary = firstNonEmpty(infoSummary(info), "原任务已不适合继续执行")
		assessment.Remediation = firstNonEmpty(infoSummary(info), "请按错误摘要处理任务对象或输入")
	case failure.ClassificationTransient:
		assessment.Eligible = true
		assessment.ReasonCode = RecoveryReasonEligible
		assessment.ReasonSummary = "失败属于可重试的外部暂时故障，恢复后将按正常策略重新排队"
	case failure.ClassificationUnknown:
		assessment.Eligible = true
		assessment.RequiresRiskConfirmation = true
		assessment.ReasonCode = RecoveryReasonUnknownConfirmation
		assessment.ReasonSummary = "无法判定失败原因，恢复后可能立即再次阻塞，也可能产生额外模型调用费用"
	}
	return assessment
}

func evaluateExecutionSafety(facts RecoveryFacts, add func(string, string, string)) {
	for _, execution := range facts.Executions {
		switch execution.Status {
		case model.AgentExecutionStatusAccepted, model.AgentExecutionStatusStarting, model.AgentExecutionStatusRunning, model.AgentExecutionStatusDispatching:
			add(RecoverySafetyExecutionActive, "前次执行仍可能在运行，不能恢复", "请等待执行结束并确认退出结果")
		}
	}
	source := sourceRun(facts)
	if source == nil {
		if !facts.DispatchProvenNotSent {
			add(RecoverySafetyExecutionUnconfirmed, "缺少前次执行已结束或未开始的受信证据", "请先完成执行对账")
		}
		return
	}
	execution := latestExecutionForRun(facts.Executions, source.RunID)
	if execution == nil {
		if !facts.DispatchProvenNotSent {
			add(RecoverySafetyExecutionUnconfirmed, "缺少前次执行的结束记录，Runner 404 或投影缺失不能证明执行已退出", "请先完成执行对账")
		}
		return
	}
	if execution.Status == model.AgentExecutionStatusInterrupted {
		add(RecoverySafetyExecutionUnconfirmed, "Runner 重启或中断没有退出证明，不能恢复", "请先确认旧执行已退出")
		return
	}
	if execution.FinishedAt == nil {
		add(RecoverySafetyExecutionUnconfirmed, "前次执行没有结束时间，不能证明已退出", "请先完成执行对账")
		return
	}
	if execution.ExecutionID != "" && execution.ExecutionID != source.RunID {
		add(RecoverySafetyExecutionUnconfirmed, "执行身份与旧 Run 不一致", "请先完成执行对账")
		return
	}
	if strings.TrimSpace(execution.RequestHash) == "" {
		add(RecoverySafetyExecutionUnconfirmed, "执行记录缺少请求身份 hash，不能证明是本次受信执行", "请先完成执行对账")
		return
	}
	if facts.ExpectedRequestHash != "" && execution.RequestHash != facts.ExpectedRequestHash {
		add(RecoverySafetyExecutionUnconfirmed, "执行请求 hash 与当前旧 Run 不一致", "请先完成执行对账")
	}
}

func evaluateExternalActions(facts RecoveryFacts, add func(string, string, string)) {
	now := facts.CheckedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for _, submission := range facts.ResultSubmissions {
		switch submission.Status {
		case model.ResultSubmissionStatusProcessing:
			add(RecoverySafetySubmissionProcessing, "存在仍在处理的结果提交", "请等待结果提交完成")
		case model.ResultSubmissionStatusUnknown, model.ResultSubmissionStatusInterrupted:
			add(RecoverySafetySubmissionUnsettled, "存在结果不明的结果提交", "请先完成结果对账")
		}
	}
	for _, writeback := range facts.Writebacks {
		switch writeback.Status {
		case model.WritebackStatusPending, model.WritebackStatusRunning:
			add(RecoverySafetyWritebackPending, "存在未完成的写回动作", "请等待写回完成")
		case model.WritebackStatusUnknown:
			add(RecoverySafetyWritebackUnknown, "存在结果不明的写回动作", "请先完成写回对账")
		}
	}
	for _, effect := range facts.Effects {
		if !effect.Critical {
			continue
		}
		switch effect.Status {
		case model.EffectStatusPending, model.EffectStatusRunning, model.EffectStatusDeliveryUnverified:
			add(RecoverySafetyCriticalEffectPending, "存在未决的关键副作用", "请先完成副作用对账")
		}
	}
	for _, admission := range facts.ActionAdmissions {
		if admission.Kind != model.ActionGitCommit && admission.Kind != model.ActionGitPush {
			continue
		}
		if admission.CompletedAt == nil && admission.NotSentAt == nil {
			add(RecoverySafetyActionUnsettled, "存在未完成或结果不明的 Git 动作", "请先完成 Git 动作对账")
		}
	}
	for _, lease := range facts.WorkItemLeases {
		if lease.LeaseUntil.After(now) {
			add(RecoverySafetyActiveExecutionLease, "存在仍有效的执行权租约，旧结果仍可能推进", "请等待租约释放")
		}
	}
	if facts.MergeGateAudit != nil && strings.TrimSpace(facts.MergeGateAudit.Decision) == "" {
		add(RecoverySafetyMergeAuditUnsettled, "存在未裁决的合并门禁记录", "请先完成合并门禁对账")
	}
}

func evaluateObjectAndRepository(facts RecoveryFacts, add func(string, string, string)) {
	if facts.TaskObjectValid != nil && !*facts.TaskObjectValid {
		add(RecoverySafetyTaskObjectInvalid, firstNonEmpty(facts.TaskObjectReason, "任务对象已失效或不完整"), "请按任务对象原因修复后重新评估")
	}
	if facts.ContextValid != nil && !*facts.ContextValid {
		add(RecoverySafetyTaskContextInvalid, firstNonEmpty(facts.ContextReason, "任务上下文无效"), "请修复任务上下文后重新评估")
	}
	if facts.ExecutionCapabilityError != "" {
		add(RecoverySafetyExecutionCapabilityFailed, facts.ExecutionCapabilityError, "请修复受控执行配置后重新评估")
	}
	if facts.PrerequisiteCheckError != "" {
		add(RecoverySafetyPrerequisiteCheckFailed, facts.PrerequisiteCheckError, "请稍后重试前置条件检查")
	}
	if facts.RepositoryLookupError != "" && facts.Repository == nil {
		code := RecoverySafetyRepositoryLookupFailed
		if facts.Task != nil && (facts.Task.RepositoryID == nil || strings.TrimSpace(*facts.Task.RepositoryID) == "") {
			code = RecoverySafetyRepositoryNotBound
		}
		add(code, facts.RepositoryLookupError, "请先完成仓库绑定并确认仓库启用")
		return
	}
	if facts.Repository == nil {
		add(RecoverySafetyRepositoryNotBound, "任务缺少注册仓库绑定", "请先绑定可用的注册仓库")
		return
	}
	if facts.Repository.Status != registeredrepo.StatusEnabled {
		add(RecoverySafetyRepositoryDisabled, "任务绑定的仓库未启用", "请先启用仓库后重新评估")
	}
}

func evaluateConflicts(facts RecoveryFacts, add func(string, string, string)) {
	if facts.HeadCheckRequired {
		if facts.HeadCheckUnavailable {
			add(RecoverySafetyHeadCheckUnavailable, "无法读取当前 PR/MR head，不能确认任务是否已过时", "请稍后重试")
		} else if facts.ExpectedHeadSHA != "" && facts.CurrentHeadSHA != "" && !strings.EqualFold(facts.ExpectedHeadSHA, facts.CurrentHeadSHA) {
			add(RecoverySafetyHeadSuperseded, "任务绑定的 head 已被新提交替代", "请创建面向新 head 的新任务")
		}
	}
	if facts.SuccessorConflict {
		add(RecoverySafetySuccessorConflict, "已存在针对同一对象新 head 的后继任务", "请处理后继任务，不要恢复旧任务")
	}
}

func sourceRunID(facts RecoveryFacts) string {
	if source := sourceRun(facts); source != nil {
		return source.RunID
	}
	return ""
}

func sourceRun(facts RecoveryFacts) *model.Run {
	if facts.Task == nil {
		return nil
	}
	if facts.Task.CurrentRunID != nil && strings.TrimSpace(*facts.Task.CurrentRunID) != "" {
		for index := range facts.Runs {
			if facts.Runs[index].RunID == *facts.Task.CurrentRunID {
				return &facts.Runs[index]
			}
		}
	}
	var latest *model.Run
	for index := range facts.Runs {
		run := &facts.Runs[index]
		if latest == nil || run.ID > latest.ID || (run.ID == latest.ID && run.CreatedAt.After(latest.CreatedAt)) {
			latest = run
		}
	}
	return latest
}

func latestExecutionForRun(executions []model.AgentExecution, runID string) *model.AgentExecution {
	var latest *model.AgentExecution
	for index := range executions {
		execution := &executions[index]
		if execution.RunID != runID {
			continue
		}
		if latest == nil || execution.ID > latest.ID {
			latest = execution
		}
	}
	return latest
}

func RecoveryBlockRevision(task *model.Task) string {
	return repository.RecoveryBlockRevision(task, "")
}

func infoSummary(info *failure.Info) string {
	if info == nil {
		return ""
	}
	return strings.TrimSpace(info.Summary)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
