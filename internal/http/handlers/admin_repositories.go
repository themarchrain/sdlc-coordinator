package handlers

import (
	"errors"
	"net/http"
	"strings"

	"sdlc-coordinator/internal/registeredrepo"

	"github.com/gin-gonic/gin"
)

type AdminRepositoryHandler struct {
	service   *registeredrepo.Service
	routeSync registeredrepo.RouteIndexSync
}

func NewAdminRepositoryHandler(service *registeredrepo.Service) *AdminRepositoryHandler {
	return &AdminRepositoryHandler{service: service}
}

// SetRouteIndexSync 注入控制面路由索引写穿端口；租户作用域
// 请求由 reposFor 按会话租户绑定写穿。
func (handler *AdminRepositoryHandler) SetRouteIndexSync(sync registeredrepo.RouteIndexSync) {
	handler.routeSync = sync
}

func (handler *AdminRepositoryHandler) reposFor(c *gin.Context) *registeredrepo.Service {
	if store, ok := TenantStoreFrom(c); ok {
		service := registeredrepo.NewService(registeredrepo.NewStore(store.DB), nil, registeredrepo.NewSQLActivityChecker(store.DB))
		if handler.routeSync != nil {
			if info := CurrentPortalSession(c); info != nil && info.EffectiveTenant != nil {
				service.SetRouteIndexSync(handler.routeSync, info.EffectiveTenant.ID)
			}
		}
		return service
	}
	return handler.service
}

func (handler *AdminRepositoryHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/repositories", handler.List)
	group.POST("/repositories", handler.Create)
	group.GET("/repositories/:repository_id", handler.Get)
	group.PATCH("/repositories/:repository_id", handler.Patch)
	group.POST("/repositories/:repository_id/disable", handler.Disable)
	group.POST("/repositories/:repository_id/enable", handler.Enable)
}

type createRepositoryBody struct {
	DisplayName         string   `json:"display_name"`
	PathWithNamespace   string   `json:"path_with_namespace"`
	Provider            string   `json:"provider"`
	BaseURL             string   `json:"base_url"`
	Token               string   `json:"token"`
	TokenScopeAttested  bool     `json:"token_scope_attested"`
	WebhookSecret       string   `json:"webhook_secret"`
	CodeHostIssueLabels []string `json:"codehost_issue_labels"`
	ReviewerMentions    string   `json:"reviewer_mentions"`
	VerifyCommand       string   `json:"verify_command"`
	GitAuthorName       string   `json:"git_author_name"`
	GitAuthorEmail      string   `json:"git_author_email"`
}

type patchRepositoryBody struct {
	DisplayName         *string   `json:"display_name"`
	PathWithNamespace   *string   `json:"path_with_namespace"`
	Provider            *string   `json:"provider"`
	BaseURL             *string   `json:"base_url"`
	Token               string    `json:"token"`
	TokenScopeAttested  *bool     `json:"token_scope_attested"`
	WebhookSecret       string    `json:"webhook_secret"`
	CodeHostIssueLabels *[]string `json:"codehost_issue_labels"`
	ReviewerMentions    *string   `json:"reviewer_mentions"`
	VerifyCommand       *string   `json:"verify_command"`
	GitAuthorName       *string   `json:"git_author_name"`
	GitAuthorEmail      *string   `json:"git_author_email"`
}

func (handler *AdminRepositoryHandler) List(c *gin.Context) {
	items, err := handler.reposFor(c).List(c.Request.Context())
	if err != nil {
		writeAdminRepositoryError(c, err)
		return
	}
	payload := make([]gin.H, 0, len(items))
	for _, item := range items {
		payload = append(payload, repositoryResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"items": payload})
}

func (handler *AdminRepositoryHandler) Create(c *gin.Context) {
	var body createRepositoryBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	view, err := handler.reposFor(c).Create(c.Request.Context(), registeredrepo.CreateRequest{
		DisplayName:         body.DisplayName,
		PathWithNamespace:   body.PathWithNamespace,
		Provider:            body.Provider,
		BaseURL:             body.BaseURL,
		Token:               body.Token,
		TokenScopeAttested:  body.TokenScopeAttested,
		WebhookSecret:       body.WebhookSecret,
		CodeHostIssueLabels: body.CodeHostIssueLabels,
		ReviewerMentions:    body.ReviewerMentions,
		VerifyCommand:       body.VerifyCommand,
		GitAuthorName:       body.GitAuthorName,
		GitAuthorEmail:      body.GitAuthorEmail,
	})
	if err != nil && view == nil {
		writeAdminRepositoryError(c, err)
		return
	}
	c.JSON(http.StatusCreated, repositoryResponse(*view))
}

func (handler *AdminRepositoryHandler) Get(c *gin.Context) {
	view, err := handler.reposFor(c).Get(c.Request.Context(), c.Param("repository_id"))
	if err != nil {
		writeAdminRepositoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, repositoryResponse(*view))
}

func (handler *AdminRepositoryHandler) Patch(c *gin.Context) {
	var body patchRepositoryBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	view, err := handler.reposFor(c).Patch(c.Request.Context(), c.Param("repository_id"), registeredrepo.PatchRequest{
		DisplayName:         body.DisplayName,
		PathWithNamespace:   body.PathWithNamespace,
		Provider:            body.Provider,
		BaseURL:             body.BaseURL,
		Token:               body.Token,
		TokenScopeAttested:  body.TokenScopeAttested,
		WebhookSecret:       body.WebhookSecret,
		CodeHostIssueLabels: body.CodeHostIssueLabels,
		ReviewerMentions:    body.ReviewerMentions,
		VerifyCommand:       body.VerifyCommand,
		GitAuthorName:       body.GitAuthorName,
		GitAuthorEmail:      body.GitAuthorEmail,
	})
	if err != nil && view == nil {
		writeAdminRepositoryError(c, err)
		return
	}
	if err != nil {
		writeAdminRepositoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, repositoryResponse(*view))
}

func (handler *AdminRepositoryHandler) Disable(c *gin.Context) {
	if err := handler.reposFor(c).Disable(c.Request.Context(), c.Param("repository_id")); err != nil {
		writeAdminRepositoryError(c, err)
		return
	}
	view, err := handler.reposFor(c).Get(c.Request.Context(), c.Param("repository_id"))
	if err != nil {
		writeAdminRepositoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, repositoryResponse(*view))
}

func (handler *AdminRepositoryHandler) Enable(c *gin.Context) {
	view, err := handler.reposFor(c).Enable(c.Request.Context(), c.Param("repository_id"))
	if err != nil {
		writeAdminRepositoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, repositoryResponse(*view))
}

func repositoryResponse(view registeredrepo.View) gin.H {
	return gin.H{
		"repository_id":             view.RepositoryID,
		"display_name":              view.DisplayName,
		"provider":                  view.Provider,
		"base_url":                  view.BaseURL,
		"codehost_project_id":       view.CodeHostProjectID,
		"path_with_namespace":       view.PathWithNamespace,
		"token_configured":          view.TokenConfigured,
		"webhook_secret_configured": view.WebhookSecretConfigured,
		"codehost_issue_labels":     view.CodeHostIssueLabels,
		"default_branch":            view.DefaultBranch,
		"status":                    view.Status,
		"last_configuration_error":  view.LastConfigurationError,
		"verify_command":            view.VerifyCommand,
		"git_author_name":           view.GitAuthorName,
		"git_author_email":          view.GitAuthorEmail,
	}
}

func writeAdminRepositoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, registeredrepo.ErrNotFound):
		writeAdminError(c, http.StatusNotFound, "repository_not_found", "仓库不存在")
	case errors.Is(err, registeredrepo.ErrTokenNotAttested):
		writeAdminError(c, http.StatusBadRequest, "token_not_attested", "请确认这是 GitLab 项目级 token")
	case errors.Is(err, registeredrepo.ErrWebhookSecretRequired):
		writeAdminError(c, http.StatusBadRequest, "webhook_secret_required", "请配置该仓库的 webhook secret（此仓事件验签的唯一凭据）")
	case errors.Is(err, registeredrepo.ErrPlatformIdentityRequired):
		writeAdminError(c, http.StatusBadRequest, "platform_identity_required", "请补齐该仓库的平台身份（平台与平台地址），启用前必须配置")
	case errors.Is(err, registeredrepo.ErrInvalidPlatformIdentity):
		writeAdminError(c, http.StatusBadRequest, "invalid_platform_identity", "平台必须是 gitlab、gitea 或 github，平台地址必须是 http(s) 地址")
	case errors.Is(err, registeredrepo.ErrRouteIdentityClaimed):
		writeAdminError(c, http.StatusConflict, "route_identity_claimed", "该仓库身份已被其他组织注册占用；路由按先注册方归属，请联系管理员裁决")
	case errors.Is(err, registeredrepo.ErrRepositoryBusy):
		writeAdminError(c, http.StatusConflict, "repository_busy", "仓库仍有活动任务，无法停用")
	case errors.Is(err, registeredrepo.ErrMustDisableFirst):
		writeAdminError(c, http.StatusConflict, "repository_must_disable_first", "请先停用仓库再更新 token 或路径")
	case errors.Is(err, registeredrepo.ErrGitIdentityRequiresDisabled):
		writeAdminError(c, http.StatusConflict, "git_identity_requires_disabled", "请先停用仓库再修改提交身份；身份修改不影响平台探活，保存后重新启用即可")
	case errors.Is(err, registeredrepo.ErrInvalidGitIdentity):
		writeAdminError(c, http.StatusUnprocessableEntity, "invalid_git_identity", "提交身份必须成对填写：姓名不超过 128 字符，邮箱为单个合法地址且不超过 254 字符，均不得含换行或控制字符")
	case errors.Is(err, registeredrepo.ErrIdentityMismatch):
		writeAdminError(c, http.StatusConflict, "repository_identity_mismatch", "GitLab 项目身份与已登记记录不一致")
	case errors.Is(err, registeredrepo.ErrInvalidRepository):
		writeAdminError(c, http.StatusBadRequest, "invalid_repository", "仓库配置不合法")
	case errors.Is(err, registeredrepo.ErrVerifyCommandTooLong):
		writeAdminError(c, http.StatusUnprocessableEntity, "verify_command_too_long", "验证命令不能超过 512 个字符")
	default:
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeAdminError(c, http.StatusConflict, "repository_conflict", "仓库路径或项目 ID 已存在")
			return
		}
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "仓库操作失败")
	}
}
