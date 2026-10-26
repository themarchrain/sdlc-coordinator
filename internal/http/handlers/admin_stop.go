package handlers

import (
	"context"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
)

type adminTaskStops interface {
	StopTask(context.Context, string, orchestrator.StopActor, string) (*orchestrator.StopResult, error)
	StopRun(context.Context, string, orchestrator.StopActor, string) (*orchestrator.StopResult, error)
	StopExecution(context.Context, string, orchestrator.StopActor, string) (*orchestrator.StopResult, error)
}

func (h *AdminOperationHandler) stopTarget(kind, idParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor := currentActor(c)
		if actor == nil || actor.ID == 0 {
			writeAdminError(c, http.StatusUnauthorized, "unauthenticated", "请先登录")
			return
		}
		operations, ok := h.operationsFor(c).(adminTaskStops)
		if !ok || operations == nil {
			writeAdminError(c, http.StatusServiceUnavailable, "operations_unavailable", "停止操作不可用")
			return
		}
		var request struct {
			Reason string `json:"reason"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		if c.Request.ContentLength != 0 && !decodeAdminConnectorBody(c, &request) {
			return
		}
		if utf8.RuneCountInString(request.Reason) > 500 {
			writeAdminError(c, http.StatusBadRequest, "invalid_body", "停止原因最多 500 个字符")
			return
		}
		stop := operations.StopTask
		if kind == "run" {
			stop = operations.StopRun
		} else if kind == "execution" {
			stop = operations.StopExecution
		}
		result, err := stop(c.Request.Context(), c.Param(idParam), orchestrator.StopActor{UserID: strconv.FormatUint(uint64(actor.ID), 10)}, request.Reason)
		if err != nil {
			writeAdminError(c, statusFromError(err), "stop_failed", "停止操作失败")
			return
		}
		status := http.StatusAccepted
		if result.DeliveryStatus == model.StopDeliveryConfirmed && result.ConfirmationKind != "" {
			status = http.StatusOK
		}
		c.JSON(status, result)
	}
}
