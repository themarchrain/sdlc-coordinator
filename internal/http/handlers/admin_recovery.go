package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/store/repository"

	"github.com/gin-gonic/gin"
)

type adminTaskRecovery interface {
	AssessTaskRecovery(context.Context, string) (orchestrator.RecoveryAssessment, error)
	RecoverBlockedTask(context.Context, string, string, orchestrator.RecoverTaskRequest) (*orchestrator.RecoverTaskResponse, error)
}

type recoverTaskBody struct {
	RequestID              string `json:"request_id"`
	BlockRevision          string `json:"block_revision"`
	Reason                 string `json:"reason"`
	AcknowledgeUnknownRisk bool   `json:"acknowledge_unknown_risk"`
}

type recoveryAssessmentResponse struct {
	TaskID                   string                       `json:"task_id"`
	Classification           string                       `json:"classification"`
	Eligible                 bool                         `json:"eligible"`
	RequiresRiskConfirmation bool                         `json:"requires_risk_confirmation"`
	ReasonCode               string                       `json:"reason_code"`
	ReasonSummary            string                       `json:"reason_summary"`
	Remediation              string                       `json:"remediation,omitempty"`
	SourceRunID              string                       `json:"source_run_id,omitempty"`
	BlockRevision            string                       `json:"block_revision"`
	FailureStage             string                       `json:"failure_stage,omitempty"`
	FailureCode              string                       `json:"failure_code,omitempty"`
	FailureSummary           string                       `json:"failure_summary,omitempty"`
	SafetyBlockers           []orchestrator.SafetyBlocker `json:"safety_blockers"`
}

func (handler *AdminOperationHandler) recoveryFor(c *gin.Context) adminTaskRecovery {
	operations := handler.operationsFor(c)
	recovery, _ := operations.(adminTaskRecovery)
	return recovery
}

func (handler *AdminOperationHandler) GetTaskRecovery(c *gin.Context) {
	recovery := handler.recoveryFor(c)
	if recovery == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "operations_unavailable", "恢复操作不可用")
		return
	}
	assessment, err := recovery.AssessTaskRecovery(c.Request.Context(), c.Param("task_id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeAdminError(c, http.StatusNotFound, "not_found", "任务不存在")
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			writeAdminError(c, http.StatusServiceUnavailable, "recovery_check_unavailable", "恢复安全检查暂不可用")
			return
		}
		writeAdminError(c, http.StatusServiceUnavailable, "recovery_check_unavailable", "恢复安全检查暂不可用")
		return
	}
	blockers := assessment.SafetyBlockers
	if blockers == nil {
		blockers = []orchestrator.SafetyBlocker{}
	}
	c.JSON(http.StatusOK, recoveryAssessmentResponse{
		TaskID: c.Param("task_id"), Classification: string(assessment.Classification),
		Eligible: assessment.Eligible, RequiresRiskConfirmation: assessment.RequiresRiskConfirmation,
		ReasonCode: assessment.ReasonCode, ReasonSummary: assessment.ReasonSummary, Remediation: assessment.Remediation,
		SourceRunID: assessment.SourceRunID, BlockRevision: assessment.BlockRevision,
		FailureStage: assessment.FailureStage, FailureCode: assessment.FailureCode, FailureSummary: assessment.FailureSummary,
		SafetyBlockers: blockers,
	})
}

func (handler *AdminOperationHandler) RecoverTask(c *gin.Context) {
	actor := currentActor(c)
	if actor == nil || actor.ID == 0 {
		writeAdminError(c, http.StatusUnauthorized, "unauthenticated", "请先登录")
		return
	}
	recovery := handler.recoveryFor(c)
	if recovery == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "operations_unavailable", "恢复操作不可用")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var body recoverTaskBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	if strings.TrimSpace(body.RequestID) == "" || strings.TrimSpace(body.BlockRevision) == "" {
		writeAdminError(c, http.StatusBadRequest, "invalid_body", "request_id 与 block_revision 必填")
		return
	}
	if utf8.RuneCountInString(body.Reason) > 1000 {
		writeAdminError(c, http.StatusBadRequest, "invalid_body", "恢复说明最多 1000 个字符")
		return
	}
	response, err := recovery.RecoverBlockedTask(c.Request.Context(), c.Param("task_id"), strconv.FormatUint(uint64(actor.ID), 10), orchestrator.RecoverTaskRequest{
		RequestID: body.RequestID, BlockRevision: body.BlockRevision, Reason: body.Reason,
		AcknowledgeUnknownRisk: body.AcknowledgeUnknownRisk,
	})
	if err != nil {
		writeRecoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"replayed": response.Replayed,
		"recovery": response.Recovery,
		"task":     taskResponse(response.Task),
	})
}

func writeRecoveryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrRecoveryRequestConflict):
		writeAdminError(c, http.StatusConflict, "recovery_request_conflict", "同一请求编号已用于其他恢复参数")
	case errors.Is(err, repository.ErrRecoveryRevisionChanged), errors.Is(err, repository.ErrRecoveryStateChanged):
		writeAdminError(c, http.StatusConflict, "task_recovery_stale", "任务状态已变化，请刷新后重试")
	case errors.Is(err, repository.ErrRecoveryNotEligible):
		writeAdminError(c, http.StatusConflict, "task_recovery_not_eligible", "当前任务不能恢复")
	case errors.Is(err, orchestrator.ErrRecoverySafetyBlocked):
		writeAdminError(c, http.StatusConflict, "task_recovery_safety_blocked", "当前任务存在未解除的安全阻塞")
	case errors.Is(err, repository.ErrRecoveryRiskRequired):
		writeAdminError(c, http.StatusBadRequest, "recovery_risk_confirmation_required", "未知失败原因必须确认风险并填写恢复说明")
	case errors.Is(err, orchestrator.ErrRecoveryInvalid), errors.Is(err, repository.ErrInvalidRecoveryCommand):
		writeAdminError(c, http.StatusBadRequest, "invalid_body", "恢复请求参数无效")
	case errors.Is(err, orchestrator.ErrRecoveryUnavailable):
		writeAdminError(c, http.StatusServiceUnavailable, "operations_unavailable", "恢复操作不可用")
	case errors.Is(err, context.DeadlineExceeded):
		writeAdminError(c, http.StatusServiceUnavailable, "recovery_check_unavailable", "恢复安全检查暂不可用")
	case errors.Is(err, repository.ErrNotFound):
		writeAdminError(c, http.StatusNotFound, "not_found", "任务不存在")
	default:
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "恢复操作失败")
	}
}
