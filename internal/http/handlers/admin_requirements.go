package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/requirements"

	"github.com/gin-gonic/gin"
)

type AdminRequirementHandler struct {
	service *requirements.Service
}

func NewAdminRequirementHandler(service *requirements.Service) *AdminRequirementHandler {
	return &AdminRequirementHandler{service: service}
}

func (handler *AdminRequirementHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/requirements", handler.List)
	group.POST("/requirements", handler.Create)
	group.POST("/requirements/sync", handler.Sync)
	group.PATCH("/requirements/:requirement_id", handler.Patch)
	group.POST("/requirements/:requirement_id/publish", handler.Publish)
	group.POST("/requirements/:requirement_id/cancel", handler.Cancel)
	group.POST("/requirements/:requirement_id/complete", handler.Complete)
}

type createRequirementBody struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	RepositoryID string   `json:"repository_id"`
	Labels       []string `json:"labels"`
	Priority     string   `json:"priority"`
}

type patchRequirementBody struct {
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	Labels      *[]string `json:"labels"`
	Priority    *string   `json:"priority"`
}

func (handler *AdminRequirementHandler) List(c *gin.Context) {
	page, pageSize := normalizeRequirementPage(c.Query("page"), c.Query("page_size"))
	items, total, err := handler.service.List(c.Request.Context(), requirements.ListFilter{
		Status:       requirements.Status(strings.ToLower(strings.TrimSpace(c.Query("status")))),
		RepositoryID: c.Query("repository_id"),
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		writeAdminRequirementError(c, err)
		return
	}
	payload := make([]gin.H, 0, len(items))
	for _, item := range items {
		payload = append(payload, requirementResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"items": payload, "page": page, "page_size": pageSize, "total": total})
}

func (handler *AdminRequirementHandler) Create(c *gin.Context) {
	var body createRequirementBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	item, err := handler.service.Create(c.Request.Context(), requirements.CreateRequest{
		Title:        body.Title,
		Description:  body.Description,
		RepositoryID: body.RepositoryID,
		Labels:       body.Labels,
		Priority:     body.Priority,
	})
	if err != nil {
		writeAdminRequirementError(c, err)
		return
	}
	c.JSON(http.StatusCreated, requirementResponse(*item))
}

func (handler *AdminRequirementHandler) Patch(c *gin.Context) {
	var body patchRequirementBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	item, err := handler.service.Patch(c.Request.Context(), c.Param("requirement_id"), requirements.PatchRequest{
		Title:       body.Title,
		Description: body.Description,
		Labels:      body.Labels,
		Priority:    body.Priority,
	})
	if err != nil {
		writeAdminRequirementError(c, err)
		return
	}
	c.JSON(http.StatusOK, requirementResponse(*item))
}

func (handler *AdminRequirementHandler) Publish(c *gin.Context) {
	item, err := handler.service.Publish(c.Request.Context(), c.Param("requirement_id"))
	if err != nil {
		writeAdminRequirementError(c, err)
		return
	}
	c.JSON(http.StatusOK, requirementResponse(*item))
}

func (handler *AdminRequirementHandler) Cancel(c *gin.Context) {
	item, err := handler.service.Cancel(c.Request.Context(), c.Param("requirement_id"))
	if err != nil {
		writeAdminRequirementError(c, err)
		return
	}
	c.JSON(http.StatusOK, requirementResponse(*item))
}

func (handler *AdminRequirementHandler) Complete(c *gin.Context) {
	item, err := handler.service.Complete(c.Request.Context(), c.Param("requirement_id"))
	if err != nil {
		writeAdminRequirementError(c, err)
		return
	}
	c.JSON(http.StatusOK, requirementResponse(*item))
}

func (handler *AdminRequirementHandler) Sync(c *gin.Context) {
	result, err := handler.service.SyncOnce(c.Request.Context())
	if err != nil {
		writeAdminRequirementError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

func normalizeRequirementPage(rawPage, rawSize string) (int, int) {
	page, err := strconv.Atoi(rawPage)
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(rawSize)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

func requirementResponse(item requirements.Requirement) gin.H {
	return gin.H{
		"requirement_id": item.RequirementID,
		"title":          item.Title,
		"description":    item.Description,
		"status":         item.Status,
		"repository_id":  item.RepositoryID,
		"labels":         item.LabelList(),
		"priority":       item.Priority,
		"issue_iid":      item.IssueIID,
		"issue_url":      item.IssueURL,
		"last_error":     item.LastError,
		"published_at":   item.PublishedAt,
		"completed_at":   item.CompletedAt,
		"created_at":     item.CreatedAt,
		"updated_at":     item.UpdatedAt,
	}
}

func writeAdminRequirementError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, requirements.ErrNotFound):
		writeAdminError(c, http.StatusNotFound, "requirement_not_found", "需求不存在")
	case errors.Is(err, registeredrepo.ErrNotFound):
		writeAdminError(c, http.StatusNotFound, "repository_not_found", "目标仓库不存在")
	case errors.Is(err, requirements.ErrRepositoryNotEnabled):
		writeAdminError(c, http.StatusBadRequest, "repository_not_enabled", "目标仓库未启用")
	case errors.Is(err, requirements.ErrInvalidRequirement):
		writeAdminError(c, http.StatusBadRequest, "invalid_requirement", "需求标题或仓库不合法")
	case errors.Is(err, requirements.ErrNotEditable):
		writeAdminError(c, http.StatusConflict, "requirement_not_editable", "当前状态不可编辑")
	case errors.Is(err, requirements.ErrInvalidTransition):
		writeAdminError(c, http.StatusConflict, "requirement_invalid_transition", "当前状态不允许该操作")
	case errors.Is(err, requirements.ErrSyncInProgress):
		writeAdminError(c, http.StatusConflict, "sync_in_progress", "需求同步正在进行中")
	default:
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "需求操作失败")
	}
}
