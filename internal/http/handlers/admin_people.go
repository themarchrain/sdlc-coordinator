package handlers

import (
	"errors"
	"net/http"
	"strings"

	"sdlc-coordinator/internal/people"

	"github.com/gin-gonic/gin"
)

type AdminPeopleHandler struct {
	service *people.Service
	lookup  people.IdentityLookup
	batch   *people.BatchLookupService
}

type personIdentityBody struct {
	IdentityID     string `json:"identity_id"`
	Provider       string `json:"provider"`
	BaseURL        string `json:"base_url"`
	ExternalUserID string `json:"external_user_id"`
	Username       string `json:"username"`
}

type createPersonBody struct {
	DisplayName string               `json:"display_name"`
	Email       string               `json:"email"`
	Enabled     *bool                `json:"enabled"`
	Identities  []personIdentityBody `json:"identities"`
}

type lookupIdentityBody struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Token    string `json:"token"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

type batchLookupIdentityBody struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Token    string `json:"token"`
}

func NewAdminPeopleHandler(service *people.Service, lookup people.IdentityLookup) *AdminPeopleHandler {
	handler := &AdminPeopleHandler{service: service, lookup: lookup}
	if lookup != nil {
		handler.batch = people.NewBatchLookupService(service, lookup)
	}
	return handler
}

func (handler *AdminPeopleHandler) peopleFor(c *gin.Context) *people.Service {
	if store, ok := TenantStoreFrom(c); ok {
		return people.NewService(people.NewRepository(store.DB))
	}
	return handler.service
}

func (handler *AdminPeopleHandler) batchFor(c *gin.Context) *people.BatchLookupService {
	if _, ok := TenantStoreFrom(c); ok && handler.lookup != nil {
		return people.NewBatchLookupService(handler.peopleFor(c), handler.lookup)
	}
	return handler.batch
}

func (handler *AdminPeopleHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/people/codehost-identities/lookup", handler.LookupIdentity)
	group.POST("/people/codehost-identities/lookup-batch", handler.LookupIdentitiesBatch)
	group.GET("/people", handler.List)
	group.POST("/people", handler.Create)
	group.GET("/people/:person_id", handler.Get)
	group.PATCH("/people/:person_id", handler.Update)
	group.DELETE("/people/:person_id", handler.Delete)
}

func (handler *AdminPeopleHandler) LookupIdentitiesBatch(c *gin.Context) {
	var body batchLookupIdentityBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	if handler.batch == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "gitlab_lookup_unavailable", "GitLab 身份查询暂不可用")
		return
	}
	result, err := handler.batchFor(c).Lookup(c.Request.Context(), people.BatchLookupRequest{
		Provider: body.Provider, BaseURL: body.BaseURL, Token: body.Token,
	})
	if err != nil {
		writeAdminPeopleError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (handler *AdminPeopleHandler) Create(c *gin.Context) {
	var body createPersonBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	person, err := handler.peopleFor(c).Create(c.Request.Context(), personRequest(body))
	if err != nil {
		writeAdminPeopleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, personResponse(*person))
}

func (handler *AdminPeopleHandler) List(c *gin.Context) {
	page, ok := parseAdminPageRequest(c)
	if !ok {
		return
	}
	enabled, ok := parseOptionalAdminBool(c, "enabled")
	if !ok {
		return
	}
	result, err := handler.peopleFor(c).List(c.Request.Context(), people.PersonFilter{
		PageRequest: people.PageRequest{Page: page.Page, PageSize: page.PageSize}, Query: c.Query("q"), Enabled: enabled,
		Provider: c.Query("provider"), BaseURL: c.Query("base_url"),
	})
	if err != nil {
		writeAdminPeopleError(c, err)
		return
	}
	items := make([]gin.H, 0, len(result.Items))
	for _, person := range result.Items {
		items = append(items, personResponse(person))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": result.Page, "page_size": result.PageSize, "total": result.Total})
}

func (handler *AdminPeopleHandler) Get(c *gin.Context) {
	person, err := handler.peopleFor(c).Get(c.Request.Context(), c.Param("person_id"))
	if err != nil {
		writeAdminPeopleError(c, err)
		return
	}
	c.JSON(http.StatusOK, personResponse(*person))
}

func (handler *AdminPeopleHandler) Update(c *gin.Context) {
	var body createPersonBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	person, err := handler.peopleFor(c).Update(c.Request.Context(), c.Param("person_id"), people.UpdatePersonRequest(personRequest(body)))
	if err != nil {
		writeAdminPeopleError(c, err)
		return
	}
	c.JSON(http.StatusOK, personResponse(*person))
}

func (handler *AdminPeopleHandler) Delete(c *gin.Context) {
	if err := handler.peopleFor(c).Delete(c.Request.Context(), c.Param("person_id")); err != nil {
		writeAdminPeopleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (handler *AdminPeopleHandler) LookupIdentity(c *gin.Context) {
	var body lookupIdentityBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	if handler.lookup == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "gitlab_lookup_unavailable", "GitLab 身份查询暂不可用")
		return
	}
	result, err := handler.lookup.Lookup(c.Request.Context(), people.LookupRequest{
		Provider: body.Provider, BaseURL: body.BaseURL, Token: body.Token, Email: body.Email, Username: body.Username,
	})
	if err != nil {
		writeAdminPeopleError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func personRequest(body createPersonBody) people.CreatePersonRequest {
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	identities := make([]people.IdentityInput, 0, len(body.Identities))
	for _, identity := range body.Identities {
		identities = append(identities, people.IdentityInput{
			IdentityID: identity.IdentityID, Provider: identity.Provider, BaseURL: identity.BaseURL,
			ExternalUserID: identity.ExternalUserID, Username: identity.Username,
		})
	}
	return people.CreatePersonRequest{DisplayName: body.DisplayName, Email: body.Email, Enabled: enabled, Identities: identities}
}

func personResponse(person people.Person) gin.H {
	identities := make([]gin.H, 0, len(person.Identities))
	for _, identity := range person.Identities {
		identities = append(identities, gin.H{
			"identity_id": identity.IdentityID, "provider": identity.Provider, "base_url": identity.CodeHostBaseURL,
			"external_user_id": identity.ExternalUserID, "username": identity.Username,
			"created_at": identity.CreatedAt, "updated_at": identity.UpdatedAt,
		})
	}
	return gin.H{
		"person_id": person.PersonID, "display_name": person.DisplayName, "email": person.Email, "enabled": person.Enabled,
		"identities": identities, "created_at": person.CreatedAt, "updated_at": person.UpdatedAt,
	}
}

func writeAdminPeopleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, people.ErrNotFound):
		writeAdminError(c, http.StatusNotFound, "not_found", "人员不存在")
	case errors.Is(err, people.ErrIncompleteEnabledPerson):
		writeAdminError(c, http.StatusBadRequest, "incomplete_codehost_identity", "启用人员前请补全 CodeHost 用户名和用户 ID")
	case errors.Is(err, people.ErrInvalidPerson), errors.Is(err, people.ErrInvalidGitLabLookup), errors.Is(err, people.ErrInvalidBatchLookup):
		writeAdminError(c, http.StatusBadRequest, "invalid_person", "人员信息或查询参数不合法")
	case errors.Is(err, people.ErrPersonEmailConflict):
		writeAdminError(c, http.StatusConflict, "person_email_conflict", "邮箱已被其他有效人员使用")
	case errors.Is(err, people.ErrCodeHostIdentityConflict):
		writeAdminError(c, http.StatusConflict, "codehost_identity_conflict", "CodeHost 身份已被其他有效人员使用")
	case errors.Is(err, people.ErrGitLabAuthFailed):
		writeAdminError(c, http.StatusUnauthorized, "gitlab_auth_failed", "GitLab Token 无效或无查询权限")
	case errors.Is(err, people.ErrGiteaAuthFailed), errors.Is(err, people.ErrGitHubAuthFailed):
		writeAdminError(c, http.StatusUnauthorized, "codehost_auth_failed", "平台 Token 无效或无查询权限")
	case errors.Is(err, people.ErrGitLabUnavailable):
		writeAdminError(c, http.StatusGatewayTimeout, "gitlab_unavailable", "GitLab 当前不可达或请求超时")
	case errors.Is(err, people.ErrGiteaUnavailable):
		writeAdminError(c, http.StatusGatewayTimeout, "gitea_unavailable", "Gitea 当前不可达或请求超时")
	case errors.Is(err, people.ErrGitHubUnavailable):
		writeAdminError(c, http.StatusGatewayTimeout, "github_unavailable", "GitHub 当前不可达或请求超时")
	case errors.Is(err, people.ErrGitLabLookupFailed):
		writeAdminError(c, http.StatusBadGateway, "gitlab_lookup_failed", "GitLab 用户查询失败")
	default:
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			writeAdminError(c, http.StatusConflict, "codehost_identity_conflict", "CodeHost 身份已被其他有效人员使用")
			return
		}
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "人员资产操作失败")
	}
}
