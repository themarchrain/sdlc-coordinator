package handlers

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"sdlc-coordinator/internal/workspaceproto"

	"github.com/gin-gonic/gin"
)

// RunnerAuthorizationService 是 Runner 受控状态改变操作（commit/push/cleanup）
// 的当前授权确认端口：任一协调器副本基于共享库裁决，旧归属一律拒绝。
type RunnerAuthorizationService interface {
	AuthorizeRunnerWorkspaceOperation(ctx context.Context, request workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error)
}

// RunnerAuthorizationHandler 是协调器侧的内部授权确认入口：部署级令牌鉴权，
// 未配置通道 fail-closed 503；内部错误同样按未确认返回（不返回细节）。
type RunnerAuthorizationHandler struct {
	service RunnerAuthorizationService
	token   string
}

func NewRunnerAuthorizationHandler(service RunnerAuthorizationService, token string) *RunnerAuthorizationHandler {
	return &RunnerAuthorizationHandler{service: service, token: strings.TrimSpace(token)}
}

func (h *RunnerAuthorizationHandler) Handle(c *gin.Context) {
	if h.service == nil || h.token == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"code": "channel_closed", "message": "runner authorization channel is not configured",
		}})
		return
	}
	const bearerPrefix = "Bearer "
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, bearerPrefix) ||
		subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(header, bearerPrefix)), []byte(h.token)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"code": "unauthorized", "message": "runner authorization requires the deployment token",
		}})
		return
	}
	var request workspaceproto.AuthorizationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_json", "message": "invalid authorization request"}})
		return
	}
	decision, err := h.service.AuthorizeRunnerWorkspaceOperation(c.Request.Context(), request)
	if err != nil {
		// 存储层不可用等内部错误按未确认处理：Runner 侧同语义 fail-closed。
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"code": "authorization_unavailable", "message": "runner authorization check failed",
		}})
		return
	}
	c.JSON(http.StatusOK, decision)
}
