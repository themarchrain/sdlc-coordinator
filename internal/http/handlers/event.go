package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"sdlc-coordinator/internal/events"

	"github.com/gin-gonic/gin"
)

// 平台身份与 endpoint 静态绑定:每个 webhook endpoint 只可能收到所属平台的事件,
// secret 查找按该 provider 作用域隔离(跨平台同 path 仓各匹配各的)。
const (
	providerGitLab = "gitlab"
	providerGitea  = "gitea"
	providerGitHub = "github"
)

// WebhookSecretLookup 按 provider + 仓路径解析该仓的 webhook secret(webhook
// secret 按仓持有,无全局回退)。返回空串表示未注册(调用方 401 拒绝);
// 返回 error 表示基础设施抖动(索引查询失败/租户库句柄不可用/仓行查询
// 故障,调用方 5xx 让代码托管重试)。两类失败都 fail-closed,未验证流量
// 不得进入事件库。
type WebhookSecretLookup interface {
	WebhookSecretByPath(ctx context.Context, provider, path string) (string, error)
}

// ProcessorFactory 按 (provider, path) 解析该仓所属租户的事件处理器。
// 返回处理器非 nil 表示命中;处理器与 error 均为零值表示未命中(回落
// 构造期的平台处理器,存量过渡期兜底);error 非 nil 表示路由已指认租户
// 但引擎不可用(组织停用/租户库打开失败),调用方 fail-closed 回 5xx 让
// 代码托管重试,不得回落平台库。
type ProcessorFactory func(ctx context.Context, provider, path string) (*events.Processor, error)

type EventHandler struct {
	processor        *events.Processor
	processorFactory ProcessorFactory
	webhookSecrets   WebhookSecretLookup
	giteaNormalizer  events.GiteaNormalizer
	githubNormalizer events.GitHubNormalizer
}

type EventHandlerOption func(*EventHandler)

// WithWebhookSecretLookup 注入按仓 secret 解析端口。注入 nil 等价于拒绝全部
// webhook 投递(fail-closed)。
func WithWebhookSecretLookup(lookup WebhookSecretLookup) EventHandlerOption {
	return func(h *EventHandler) {
		h.webhookSecrets = lookup
	}
}

// WithReworkAllowedUserIDs 注入平台级 rework 白名单(平台 user id),
// gitea/github normalizer 与 GitLab 侧共用同一份配置。
func WithReworkAllowedUserIDs(ids []int) EventHandlerOption {
	return func(h *EventHandler) {
		h.giteaNormalizer = events.NewGiteaNormalizer(events.WithGiteaReworkAllowedUserIDs(ids))
		h.githubNormalizer = events.NewGitHubNormalizer(events.WithGitHubReworkAllowedUserIDs(ids))
	}
}

// WithProcessorFactory 注入按仓路由的处理器工厂;每个 webhook 请求在验签
// 通过后按 (provider, path) 选择处理器实例(租户引擎;未命中回落平台兜底,
// 引擎不可用则拒绝)。
func WithProcessorFactory(factory ProcessorFactory) EventHandlerOption {
	return func(h *EventHandler) {
		h.processorFactory = factory
	}
}

// resolveProcessor 选取本请求的事件处理器:工厂命中用租户处理器;路由已
// 指认租户但引擎不可用则上抛错误(调用方 5xx 让代码托管重试,不回落
// 平台库);仅未命中回落构造期平台实例(存量过渡期兜底)。
func (h *EventHandler) resolveProcessor(ctx context.Context, provider, path string) (*events.Processor, error) {
	if h.processorFactory != nil {
		processor, err := h.processorFactory(ctx, provider, path)
		if err != nil {
			return nil, err
		}
		if processor != nil {
			return processor, nil
		}
	}
	return h.processor, nil
}

type GitLabEventResponse struct {
	EventID      string        `json:"event_id"`
	EventStatus  string        `json:"event_status"`
	Ignored      bool          `json:"ignored"`
	IgnoreReason string        `json:"ignore_reason,omitempty"`
	Duplicate    bool          `json:"duplicate"`
	Task         *TaskResponse `json:"task,omitempty"`
}

func NewEventHandler(processor *events.Processor, opts ...EventHandlerOption) *EventHandler {
	handler := &EventHandler{processor: processor}
	for _, opt := range opts {
		opt(handler)
	}
	return handler
}

func (h *EventHandler) GitLab(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "read gitlab event payload failed",
			"details": err.Error(),
		})
		return
	}
	// 鉴权先于业务依赖检查:先从不可信 payload 最小解析仓路径,取该仓 secret 验签;
	// 未授权流量不暴露服务内部状态。
	path, ok := gitLabRepoPath(raw)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "malformed gitlab webhook payload"})
		return
	}
	authorized, authErr := h.authorizeGitLabWebhook(c.Request.Context(), path, c.GetHeader("X-Gitlab-Token"))
	if authErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret lookup unavailable"})
		return
	}
	if !authorized {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized gitlab webhook"})
		return
	}
	processor, err := h.resolveProcessor(c.Request.Context(), providerGitLab, path)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event processor unavailable"})
		return
	}
	if h == nil || processor == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "event processor not configured"})
		return
	}

	result, err := processor.ProcessGitLabWebhook(c.Request.Context(), raw, events.WithGitLabWebhookMetadata(events.GitLabWebhookMetadata{
		EventName:   c.GetHeader("X-Gitlab-Event"),
		EventUUID:   c.GetHeader("X-Gitlab-Event-UUID"),
		WebhookUUID: c.GetHeader("X-Gitlab-Webhook-UUID"),
		InstanceURL: c.GetHeader("X-Gitlab-Instance"),
	}))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "process gitlab event failed",
			"details": err.Error(),
		})
		return
	}

	resp := GitLabEventResponse{
		Ignored:      result.Ignored,
		IgnoreReason: result.IgnoreReason,
		Duplicate:    result.Duplicate,
	}
	if result.Event != nil {
		resp.EventID = result.Event.EventID
		resp.EventStatus = string(result.Event.Status)
	}
	if result.Task != nil {
		task := taskResponse(result.Task)
		resp.Task = &task
	}

	c.JSON(http.StatusAccepted, resp)
}

// Gitea 处理 Gitea webhook:X-Gitea-Signature 携带 hex(hmac_sha256(secret, body)),
// secret 按仓解析,使用常量时间比较;事件经 GiteaNormalizer 规范化后进入统一编排。
func (h *EventHandler) Gitea(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "read gitea event payload failed",
			"details": err.Error(),
		})
		return
	}
	// 鉴权先于业务依赖检查:最小解析仓路径 → 取该仓 secret → 验签。
	path, ok := codeHostRepoPath(raw)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "malformed gitea webhook payload"})
		return
	}
	authorized, authErr := h.authorizeCodeHostWebhook(c.Request.Context(), providerGitea, path, c.GetHeader("X-Gitea-Signature"), raw, "")
	if authErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret lookup unavailable"})
		return
	}
	if !authorized {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized gitea webhook"})
		return
	}
	processor, err := h.resolveProcessor(c.Request.Context(), providerGitea, path)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event processor unavailable"})
		return
	}
	if h == nil || processor == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "event processor not configured"})
		return
	}

	result, err := processor.ProcessWebhook(c.Request.Context(), h.giteaNormalizer, raw, events.WebhookMetadata{
		EventType:   c.GetHeader("X-Gitea-Event"),
		DeliveryID:  c.GetHeader("X-Gitea-Delivery"),
		InstanceURL: c.GetHeader("X-Gitea-Instance"),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "process gitea event failed",
			"details": err.Error(),
		})
		return
	}

	resp := GitLabEventResponse{
		Ignored:      result.Ignored,
		IgnoreReason: result.IgnoreReason,
		Duplicate:    result.Duplicate,
	}
	if result.Event != nil {
		resp.EventID = result.Event.EventID
		resp.EventStatus = string(result.Event.Status)
	}
	if result.Task != nil {
		task := taskResponse(result.Task)
		resp.Task = &task
	}

	c.JSON(http.StatusAccepted, resp)
}

// GitHub 处理 GitHub webhook:X-Hub-Signature-256 携带 "sha256=" 前缀的
// hex(hmac_sha256(secret, body)),secret 按仓解析,常量时间比较;事件经
// GitHubNormalizer 规范化后进入统一编排。
func (h *EventHandler) GitHub(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "read github event payload failed",
			"details": err.Error(),
		})
		return
	}
	// 鉴权先于业务依赖检查:最小解析仓路径 → 取该仓 secret → 验签。
	path, ok := codeHostRepoPath(raw)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "malformed github webhook payload"})
		return
	}
	authorized, authErr := h.authorizeCodeHostWebhook(c.Request.Context(), providerGitHub, path, c.GetHeader("X-Hub-Signature-256"), raw, "sha256=")
	if authErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret lookup unavailable"})
		return
	}
	if !authorized {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized github webhook"})
		return
	}
	processor, err := h.resolveProcessor(c.Request.Context(), providerGitHub, path)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event processor unavailable"})
		return
	}
	if h == nil || processor == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "event processor not configured"})
		return
	}

	result, err := processor.ProcessWebhook(c.Request.Context(), h.githubNormalizer, raw, events.WebhookMetadata{
		EventType:   c.GetHeader("X-GitHub-Event"),
		DeliveryID:  c.GetHeader("X-GitHub-Delivery"),
		InstanceURL: c.GetHeader("X-GitHub-Enterprise-Host"),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "process github event failed",
			"details": err.Error(),
		})
		return
	}

	resp := GitLabEventResponse{
		Ignored:      result.Ignored,
		IgnoreReason: result.IgnoreReason,
		Duplicate:    result.Duplicate,
	}
	if result.Event != nil {
		resp.EventID = result.Event.EventID
		resp.EventStatus = string(result.Event.Status)
	}
	if result.Task != nil {
		task := taskResponse(result.Task)
		resp.Task = &task
	}

	c.JSON(http.StatusAccepted, resp)
}

// repoWebhookSecret 解析仓 secret；err 非 nil 表示基础设施抖动（如租户库
// 句柄不可用），调用方回 5xx 让代码托管重试，不与「未注册」（空串）混淆。
func (h *EventHandler) repoWebhookSecret(ctx context.Context, provider, path string) (string, error) {
	if h == nil || h.webhookSecrets == nil {
		return "", nil
	}
	secret, err := h.webhookSecrets.WebhookSecretByPath(ctx, provider, path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(secret), nil
}

func (h *EventHandler) authorizeGitLabWebhook(ctx context.Context, path, received string) (bool, error) {
	expected, err := h.repoWebhookSecret(ctx, providerGitLab, path)
	if err != nil {
		return false, err
	}
	// 该仓未注册或未配置 secret 一律拒绝(fail-closed):未验证流量不得进入事件库。
	if expected == "" {
		return false, nil
	}
	actual := strings.TrimSpace(received)
	if actual == "" || len(actual) != len(expected) {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1, nil
}

// authorizeCodeHostWebhook 验证 HMAC 签名(Gitea 与 GitHub 共用,差异只在
// provider 作用域与 signaturePrefix:Gitea 裸 hex,GitHub 带 "sha256=" 前缀)。
func (h *EventHandler) authorizeCodeHostWebhook(ctx context.Context, provider, path, receivedSignature string, body []byte, signaturePrefix string) (bool, error) {
	expected, err := h.repoWebhookSecret(ctx, provider, path)
	if err != nil {
		return false, err
	}
	// 该仓未注册或未配置 secret 一律拒绝(fail-closed):未验证流量不得进入事件库。
	if expected == "" {
		return false, nil
	}
	received := strings.TrimSpace(receivedSignature)
	if signaturePrefix != "" {
		// GitHub 固定发送前缀;前缀缺失或类型不符直接拒绝。
		if !strings.HasPrefix(strings.ToLower(received), signaturePrefix) {
			return false, nil
		}
		received = received[len(signaturePrefix):]
	}
	if received == "" {
		return false, nil
	}
	mac := hmac.New(sha256.New, []byte(expected))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(received), []byte(expectedMAC)), nil
}

// gitLabRepoPath 最小解析 project.path_with_namespace;只读取这一个字段用于
// 定位仓,不信任 payload 其余内容。
func gitLabRepoPath(raw []byte) (string, bool) {
	var payload struct {
		Project struct {
			PathWithNamespace string `json:"path_with_namespace"`
		} `json:"project"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", false
	}
	path := strings.TrimSpace(payload.Project.PathWithNamespace)
	if path == "" {
		return "", false
	}
	return path, true
}

// codeHostRepoPath 最小解析 repository.full_name(Gitea/GitHub 载荷的仓路径键)。
func codeHostRepoPath(raw []byte) (string, bool) {
	var payload struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", false
	}
	path := strings.TrimSpace(payload.Repository.FullName)
	if path == "" {
		return "", false
	}
	return path, true
}
