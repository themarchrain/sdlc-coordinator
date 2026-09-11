package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
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
// secret 按仓持有,无全局回退)。repoURL 是载荷自带的仓库完整链接
// (GitLab=project.web_url,Gitea/GitHub=repository.html_url),仅在
// (provider, path) 命中多行时用于推导实例地址消歧,单行命中与未命中不
// 参与逻辑。返回空串表示未注册(调用方 401 拒绝);返回 error 表示基础
// 设施抖动(索引查询失败/租户库句柄不可用/仓行查询故障,调用方 5xx 让
// 代码托管重试)。两类失败都 fail-closed,未验证流量不得进入事件库。
type WebhookSecretLookup interface {
	WebhookSecretByPath(ctx context.Context, provider, path, repoURL string) (string, error)
}

// WebhookEngineView 一次 webhook 请求所需的引擎件:处理器与按当前组织段
// 配置构建的平台 normalizer(GitLab normalizer 由处理器按构造期选项现建,
// 不在本视图内)。
type WebhookEngineView struct {
	Processor        *events.Processor
	GiteaNormalizer  events.GiteaNormalizer
	GitHubNormalizer events.GitHubNormalizer
}

// EngineViewFactory 按 (provider, path) 解析该仓所属租户的引擎视图;
// repoURL 同 WebhookSecretLookup,仅多行命中时参与实例消歧。返回视图非 nil
// 表示命中;error 非 nil 表示引擎不可用(组织停用/租户库打开失败/路径歧义
// 无法消解/租户配置冷读失败/路由缺失),调用方 fail-closed 回 5xx 让代码
// 托管重试。仓库只经组织注册,不存在平台级兜底视图。
type EngineViewFactory func(ctx context.Context, provider, path, repoURL string) (*WebhookEngineView, error)

type EventHandler struct {
	platformView      *WebhookEngineView
	engineViewFactory EngineViewFactory
	webhookSecrets    WebhookSecretLookup
}

type EventHandlerOption func(*EventHandler)

// WithWebhookSecretLookup 注入按仓 secret 解析端口。注入 nil 等价于拒绝全部
// webhook 投递(fail-closed)。
func WithWebhookSecretLookup(lookup WebhookSecretLookup) EventHandlerOption {
	return func(h *EventHandler) {
		h.webhookSecrets = lookup
	}
}

// WithEngineViewFactory 注入按仓路由的引擎视图工厂;每个 webhook 请求在验签
// 通过后按 (provider, path) 选择租户引擎视图,引擎不可用则拒绝,视图内的
// normalizer 按当前组织段配置构建,白名单热生效。
func WithEngineViewFactory(factory EngineViewFactory) EventHandlerOption {
	return func(h *EventHandler) {
		h.engineViewFactory = factory
	}
}

// errEngineViewMissing 表示已注入工厂却返回零值视图:装配契约违例,按引擎
// 不可用拒绝,绝不静默回落构造期默认视图把事件写进错误的库。
var errEngineViewMissing = errors.New("event engine view missing")

// resolveEngineView 选取本请求的引擎视图:工厂命中用租户视图;工厂返回
// 错误或零值视图一律上抛(调用方 5xx 让代码托管重试)。未注入工厂的装配
// (测试/引导形态)沿用构造期默认视图。
func (h *EventHandler) resolveEngineView(ctx context.Context, provider, path, repoURL string) (*WebhookEngineView, error) {
	if h.engineViewFactory != nil {
		view, err := h.engineViewFactory(ctx, provider, path, repoURL)
		if err != nil {
			return nil, err
		}
		if view != nil {
			return view, nil
		}
		return nil, errEngineViewMissing
	}
	return h.platformView, nil
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
	handler := &EventHandler{platformView: &WebhookEngineView{
		Processor:        processor,
		GiteaNormalizer:  events.NewGiteaNormalizer(),
		GitHubNormalizer: events.NewGitHubNormalizer(),
	}}
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
	repoURL := gitLabRepoWebURL(raw)
	authorized, authErr := h.authorizeGitLabWebhook(c.Request.Context(), path, repoURL, c.GetHeader("X-Gitlab-Token"))
	if authErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret lookup unavailable"})
		return
	}
	if !authorized {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized gitlab webhook"})
		return
	}
	view, err := h.resolveEngineView(c.Request.Context(), providerGitLab, path, repoURL)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event processor unavailable"})
		return
	}
	if view == nil || view.Processor == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "event processor not configured"})
		return
	}

	result, err := view.Processor.ProcessGitLabWebhook(c.Request.Context(), raw, events.WithGitLabWebhookMetadata(events.GitLabWebhookMetadata{
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
	repoURL := codeHostRepoHTMLURL(raw)
	authorized, authErr := h.authorizeCodeHostWebhook(c.Request.Context(), providerGitea, path, repoURL, c.GetHeader("X-Gitea-Signature"), raw, "")
	if authErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret lookup unavailable"})
		return
	}
	if !authorized {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized gitea webhook"})
		return
	}
	view, err := h.resolveEngineView(c.Request.Context(), providerGitea, path, repoURL)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event processor unavailable"})
		return
	}
	if view == nil || view.Processor == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "event processor not configured"})
		return
	}

	result, err := view.Processor.ProcessWebhook(c.Request.Context(), view.GiteaNormalizer, raw, events.WebhookMetadata{
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
	repoURL := codeHostRepoHTMLURL(raw)
	authorized, authErr := h.authorizeCodeHostWebhook(c.Request.Context(), providerGitHub, path, repoURL, c.GetHeader("X-Hub-Signature-256"), raw, "sha256=")
	if authErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret lookup unavailable"})
		return
	}
	if !authorized {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized github webhook"})
		return
	}
	view, err := h.resolveEngineView(c.Request.Context(), providerGitHub, path, repoURL)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event processor unavailable"})
		return
	}
	if view == nil || view.Processor == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "event processor not configured"})
		return
	}

	result, err := view.Processor.ProcessWebhook(c.Request.Context(), view.GitHubNormalizer, raw, events.WebhookMetadata{
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
// repoURL 仅在路由索引多行命中时被查找端用于实例消歧。
func (h *EventHandler) repoWebhookSecret(ctx context.Context, provider, path, repoURL string) (string, error) {
	if h == nil || h.webhookSecrets == nil {
		return "", nil
	}
	secret, err := h.webhookSecrets.WebhookSecretByPath(ctx, provider, path, repoURL)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(secret), nil
}

func (h *EventHandler) authorizeGitLabWebhook(ctx context.Context, path, repoURL, received string) (bool, error) {
	expected, err := h.repoWebhookSecret(ctx, providerGitLab, path, repoURL)
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
func (h *EventHandler) authorizeCodeHostWebhook(ctx context.Context, provider, path, repoURL, receivedSignature string, body []byte, signaturePrefix string) (bool, error) {
	expected, err := h.repoWebhookSecret(ctx, provider, path, repoURL)
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

// gitLabRepoWebURL 最小解析 project.web_url:仅在 (provider, path) 命中
// 多行时用于推导实例地址消歧,缺失返回空串(消歧侧据此维持 fail-closed)。
func gitLabRepoWebURL(raw []byte) string {
	var payload struct {
		Project struct {
			WebURL string `json:"web_url"`
		} `json:"project"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Project.WebURL)
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

// codeHostRepoHTMLURL 最小解析 repository.html_url(Gitea/GitHub 载荷的
// 仓库完整链接):仅在多行命中时用于推导实例地址消歧,缺失返回空串。
func codeHostRepoHTMLURL(raw []byte) string {
	var payload struct {
		Repository struct {
			HTMLURL string `json:"html_url"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Repository.HTMLURL)
}

// DeriveInstanceBaseURL 从载荷自带的仓库完整链接推导实例地址。三个平台
// 的完整链接均为 实例地址 + "/" + 仓路径 的拼接形态,且仓路径已另行解析
// 在手,直接切后缀最稳;推导结果交给注册侧同一个 NormalizeBaseURL 规整
// 后再与路由行比对(调用方职责)。链接为空或后缀不匹配时返回 false,
// 调用方维持原有的歧义拒绝,不猜不兜底。
func DeriveInstanceBaseURL(repoURL, repoPath string) (string, bool) {
	repoURL = strings.TrimSpace(repoURL)
	suffix := "/" + repoPath
	if repoURL == "" || !strings.HasSuffix(repoURL, suffix) {
		return "", false
	}
	return strings.TrimSuffix(repoURL, suffix), true
}
