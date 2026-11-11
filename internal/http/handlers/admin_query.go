package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store/repository"

	"github.com/gin-gonic/gin"
)

type AdminQueryHandler struct {
	queries *admin.QueryService
}

func NewAdminQueryHandler(queries *admin.QueryService) *AdminQueryHandler {
	return &AdminQueryHandler{queries: queries}
}

func (handler *AdminQueryHandler) queriesFor(c *gin.Context) *admin.QueryService {
	if store, ok := TenantStoreFrom(c); ok {
		return admin.NewQueryService(store.DB)
	}
	return handler.queries
}

func (handler *AdminQueryHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/overview", handler.Overview)
	group.GET("/metrics", handler.Metrics)
	group.GET("/tasks", handler.ListTasks)
	group.GET("/tasks/:task_id/timeline", handler.TaskTimeline)
	group.GET("/tasks/:task_id", handler.GetTask)
	group.GET("/runs", handler.ListRuns)
	group.GET("/runs/:run_id", handler.GetRun)
	group.GET("/artifacts", handler.ListArtifacts)
	group.GET("/artifacts/:artifact_id", handler.GetArtifact)
	group.GET("/dependencies", handler.ListDependencies)
	group.GET("/dependencies/:dependency_id", handler.GetDependency)
	group.GET("/writebacks", handler.ListWritebacks)
	group.GET("/writebacks/:writeback_id", handler.GetWriteback)
	group.GET("/effects", handler.ListEffects)
	group.GET("/effects/:effect_id", handler.GetEffect)
	group.GET("/inbound-events", handler.ListInboundEvents)
	group.GET("/inbound-events/:event_id", handler.GetInboundEvent)
	group.GET("/result-submissions", handler.ListResultSubmissions)
	group.GET("/result-submissions/:submission_id", handler.GetResultSubmission)
}

func (handler *AdminQueryHandler) Metrics(c *gin.Context) {
	window, err := admin.ParseMetricsWindow(c.Query("window"))
	if err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_query", "window 必须是 7d、30d 或 all")
		return
	}
	metrics, err := handler.queriesFor(c).Metrics(c.Request.Context(), admin.MetricsFilter{
		Window:       window,
		Type:         strings.TrimSpace(c.Query("type")),
		RepositoryID: strings.TrimSpace(c.Query("repository_id")),
		Now:          time.Now().UTC(),
	})
	if err != nil {
		writeAdminQueryServiceError(c, err, "自动化指标")
		return
	}
	c.JSON(http.StatusOK, automationMetricsResponse(metrics))
}

func (handler *AdminQueryHandler) Overview(c *gin.Context) {
	window, err := admin.ParseMetricsWindow(c.Query("window"))
	if err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_query", "window 必须是 7d、30d 或 all")
		return
	}
	repositoryID := strings.TrimSpace(c.Query("repository_id"))
	overview, err := handler.queriesFor(c).Overview(c.Request.Context(), admin.OverviewFilter{RepositoryID: repositoryID})
	if err != nil {
		writeAdminQueryServiceError(c, err, "Overview")
		return
	}
	business, err := handler.queriesFor(c).BusinessSummary(c.Request.Context(), admin.MetricsFilter{
		Window: window, RepositoryID: repositoryID, Now: time.Now().UTC(),
	})
	if err != nil {
		writeAdminQueryServiceError(c, err, "业务结果摘要")
		return
	}
	c.JSON(http.StatusOK, overviewResponse(overview, business))
}

func (handler *AdminQueryHandler) TaskTimeline(c *gin.Context) {
	events, err := handler.queriesFor(c).TaskTimeline(c.Request.Context(), c.Param("task_id"))
	if err != nil {
		writeAdminQueryLookupError(c, err, "Task")
		return
	}
	items := make([]gin.H, 0, len(events))
	for _, event := range events {
		items = append(items, timelineEventResponse(event))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (handler *AdminQueryHandler) ListTasks(c *gin.Context) {
	pageRequest, from, to, ok := parseAdminListQuery(c)
	if !ok {
		return
	}

	page, err := handler.queriesFor(c).ListTasks(c.Request.Context(), admin.TaskFilter{
		PageRequest:  pageRequest,
		Status:       c.Query("status"),
		Role:         c.Query("role"),
		Type:         c.Query("type"),
		Repository:   c.Query("repository"),
		RepositoryID: c.Query("repository_id"),
		CreatedFrom:  from,
		CreatedTo:    to,
	})
	if err != nil {
		writeAdminQueryServiceError(c, err, "Task")
		return
	}

	items := make([]gin.H, 0, len(page.Items))
	for _, task := range page.Items {
		items = append(items, taskSummary(task))
	}
	c.JSON(http.StatusOK, gin.H{
		"items": items, "page": page.Page, "page_size": page.PageSize, "total": page.Total,
	})
}

func (handler *AdminQueryHandler) GetTask(c *gin.Context) {
	task, err := handler.queriesFor(c).GetTask(c.Request.Context(), c.Param("task_id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeAdminError(c, http.StatusNotFound, "not_found", "Task 不存在")
			return
		}
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "查询 Task 失败")
		return
	}
	stop, err := handler.queriesFor(c).StopStatus(c.Request.Context(), repository.StopTarget{TaskID: task.TaskID})
	if err != nil {
		writeAdminQueryServiceError(c, err, "停止状态")
		return
	}
	response := taskDetail(*task)
	allowed, err := handler.queriesFor(c).CanStop(c.Request.Context(), repository.StopTarget{TaskID: task.TaskID})
	if err != nil {
		writeAdminQueryServiceError(c, err, "停止权限")
		return
	}
	response["can_stop"] = allowed
	response["stop"] = stop
	c.JSON(http.StatusOK, response)
}

func (handler *AdminQueryHandler) ListRuns(c *gin.Context) {
	pageRequest, from, to, ok := parseAdminListQuery(c)
	if !ok {
		return
	}
	page, err := handler.queriesFor(c).ListRuns(c.Request.Context(), admin.RunFilter{
		PageRequest: pageRequest, Status: c.Query("status"), WorkerRole: c.Query("worker_role"),
		TaskID: c.Query("task_id"), RepositoryID: c.Query("repository_id"), CreatedFrom: from, CreatedTo: to,
	})
	writeAdminQueryPage(c, page, err, runSummary, "Run")
}

func (handler *AdminQueryHandler) GetRun(c *gin.Context) {
	entity, err := handler.queriesFor(c).GetRun(c.Request.Context(), c.Param("run_id"))
	if err != nil {
		writeAdminQueryLookupError(c, err, "Run")
		return
	}
	stop, err := handler.queriesFor(c).StopStatus(c.Request.Context(), repository.StopTarget{RunID: entity.RunID})
	if err != nil {
		writeAdminQueryServiceError(c, err, "停止状态")
		return
	}
	response := runDetail(*entity)
	allowed, err := handler.queriesFor(c).CanStop(c.Request.Context(), repository.StopTarget{RunID: entity.RunID})
	if err != nil {
		writeAdminQueryServiceError(c, err, "停止权限")
		return
	}
	response["can_stop"] = allowed
	response["stop"] = stop
	c.JSON(http.StatusOK, response)
}

func (handler *AdminQueryHandler) ListArtifacts(c *gin.Context) {
	filter, ok := parseAdminEntityFilter(c)
	if !ok {
		return
	}
	page, err := handler.queriesFor(c).ListArtifacts(c.Request.Context(), filter)
	writeAdminQueryPage(c, page, err, artifactSummary, "Artifact")
}

func (handler *AdminQueryHandler) GetArtifact(c *gin.Context) {
	entity, err := handler.queriesFor(c).GetArtifact(c.Request.Context(), c.Param("artifact_id"))
	writeAdminQueryDetail(c, entity, err, artifactDetail, "Artifact")
}

func (handler *AdminQueryHandler) ListDependencies(c *gin.Context) {
	pageRequest, from, to, ok := parseAdminListQuery(c)
	if !ok {
		return
	}
	page, err := handler.queriesFor(c).ListDependencies(c.Request.Context(), admin.DependencyFilter{
		PageRequest: pageRequest, ParentTaskID: c.Query("parent_task_id"), ChildTaskID: c.Query("child_task_id"),
		Relation: c.Query("relation"), Status: c.Query("status"), RepositoryID: c.Query("repository_id"),
		CreatedFrom: from, CreatedTo: to,
	})
	writeAdminQueryPage(c, page, err, dependencySummary, "Dependency")
}

func (handler *AdminQueryHandler) GetDependency(c *gin.Context) {
	entity, err := handler.queriesFor(c).GetDependency(c.Request.Context(), c.Param("dependency_id"))
	writeAdminQueryDetail(c, entity, err, dependencyDetail, "Dependency")
}

func (handler *AdminQueryHandler) ListWritebacks(c *gin.Context) {
	filter, ok := parseAdminEntityFilter(c)
	if !ok {
		return
	}
	page, err := handler.queriesFor(c).ListWritebacks(c.Request.Context(), filter)
	writeAdminQueryPage(c, page, err, writebackSummary, "Writeback")
}

func (handler *AdminQueryHandler) GetWriteback(c *gin.Context) {
	entity, err := handler.queriesFor(c).GetWriteback(c.Request.Context(), c.Param("writeback_id"))
	writeAdminQueryDetail(c, entity, err, writebackDetail, "Writeback")
}

func (handler *AdminQueryHandler) ListEffects(c *gin.Context) {
	filter, ok := parseAdminEntityFilter(c)
	if !ok {
		return
	}
	page, err := handler.queriesFor(c).ListEffects(c.Request.Context(), filter)
	writeAdminQueryPage(c, page, err, effectSummary, "Effect")
}

func (handler *AdminQueryHandler) GetEffect(c *gin.Context) {
	entity, err := handler.queriesFor(c).GetEffect(c.Request.Context(), c.Param("effect_id"))
	writeAdminQueryDetail(c, entity, err, effectDetail, "Effect")
}

func (handler *AdminQueryHandler) ListInboundEvents(c *gin.Context) {
	filter, ok := parseAdminEntityFilter(c)
	if !ok {
		return
	}
	page, err := handler.queriesFor(c).ListInboundEvents(c.Request.Context(), filter)
	writeAdminQueryPage(c, page, err, inboundEventSummary, "InboundEvent")
}

func (handler *AdminQueryHandler) GetInboundEvent(c *gin.Context) {
	entity, err := handler.queriesFor(c).GetInboundEvent(c.Request.Context(), c.Param("event_id"))
	writeAdminQueryDetail(c, entity, err, inboundEventDetail, "InboundEvent")
}

func (handler *AdminQueryHandler) ListResultSubmissions(c *gin.Context) {
	filter, ok := parseAdminEntityFilter(c)
	if !ok {
		return
	}
	page, err := handler.queriesFor(c).ListResultSubmissions(c.Request.Context(), filter)
	writeAdminQueryPage(c, page, err, resultSubmissionSummary, "ResultSubmission")
}

func (handler *AdminQueryHandler) GetResultSubmission(c *gin.Context) {
	entity, err := handler.queriesFor(c).GetResultSubmission(c.Request.Context(), c.Param("submission_id"))
	writeAdminQueryDetail(c, entity, err, resultSubmissionDetail, "ResultSubmission")
}

func parseAdminPageRequest(c *gin.Context) (admin.PageRequest, bool) {
	page, ok := parsePositiveAdminInt(c, "page")
	if !ok {
		return admin.PageRequest{}, false
	}
	pageSize, ok := parsePositiveAdminInt(c, "page_size")
	if !ok {
		return admin.PageRequest{}, false
	}
	return admin.PageRequest{Page: page, PageSize: pageSize}, true
}

func parseAdminListQuery(c *gin.Context) (admin.PageRequest, *time.Time, *time.Time, bool) {
	pageRequest, ok := parseAdminPageRequest(c)
	if !ok {
		return admin.PageRequest{}, nil, nil, false
	}
	from, ok := parseAdminRFC3339(c, "created_from")
	if !ok {
		return admin.PageRequest{}, nil, nil, false
	}
	to, ok := parseAdminRFC3339(c, "created_to")
	if !ok {
		return admin.PageRequest{}, nil, nil, false
	}
	return pageRequest, from, to, true
}

func parseAdminEntityFilter(c *gin.Context) (admin.EntityFilter, bool) {
	pageRequest, from, to, ok := parseAdminListQuery(c)
	if !ok {
		return admin.EntityFilter{}, false
	}
	return admin.EntityFilter{
		PageRequest: pageRequest, TaskID: c.Query("task_id"), RunID: c.Query("run_id"),
		Status: c.Query("status"), Kind: c.Query("kind"), Source: c.Query("source"),
		Type: c.Query("type"), Role: c.Query("role"), RepositoryID: c.Query("repository_id"),
		CreatedFrom: from, CreatedTo: to,
	}, true
}

func parseAdminRFC3339(c *gin.Context, name string) (*time.Time, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, true
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_query", name+" 必须是 RFC3339 时间")
		return nil, false
	}
	return &value, true
}

func parsePositiveAdminInt(c *gin.Context, name string) (int, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return 0, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		writeAdminError(c, http.StatusBadRequest, "invalid_query", name+" 必须是正整数")
		return 0, false
	}
	return value, true
}

func writeAdminError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{
		"code": code, "message": message, "request_id": "", "details": nil,
	}})
}

func writeAdminQueryServiceError(c *gin.Context, err error, entityName string) {
	if errors.Is(err, registeredrepo.ErrNotFound) {
		writeAdminError(c, http.StatusNotFound, "repository_not_found", "仓库不存在")
		return
	}
	writeAdminError(c, http.StatusInternalServerError, "internal_error", "查询 "+entityName+" 失败")
}

func writeAdminQueryLookupError(c *gin.Context, err error, entityName string) {
	if errors.Is(err, repository.ErrNotFound) {
		writeAdminError(c, http.StatusNotFound, "not_found", entityName+" 不存在")
		return
	}
	writeAdminQueryServiceError(c, err, entityName)
}

func writeAdminQueryPage[T any](c *gin.Context, page admin.Page[T], err error, mapper func(T) gin.H, entityName string) {
	if err != nil {
		writeAdminQueryServiceError(c, err, entityName)
		return
	}
	items := make([]gin.H, 0, len(page.Items))
	for _, entity := range page.Items {
		items = append(items, mapper(entity))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": page.Page, "page_size": page.PageSize, "total": page.Total})
}

func writeAdminQueryDetail[T any](c *gin.Context, entity *T, err error, mapper func(T) gin.H, entityName string) {
	if err != nil {
		writeAdminQueryLookupError(c, err, entityName)
		return
	}
	c.JSON(http.StatusOK, mapper(*entity))
}

func taskSummary(task model.Task) gin.H {
	return gin.H{
		"task_id": task.TaskID, "dedupe_key": task.DedupeKey, "type": task.Type, "role": task.Role,
		"source_repo": task.SourceRepo, "target_repo": task.TargetRepo, "repository_id": task.RepositoryID,
		"status":      task.Status,
		"lease_until": task.LeaseUntil, "attempt_count": task.AttemptCount, "current_run_id": task.CurrentRunID,
		"recovery_generation": task.RecoveryGeneration, "retry_cycle_base_count": task.RetryCycleBaseCount,
		"retry_cycle_max_attempts": task.RetryCycleMaxAttempts, "retry_cycle_attempt_count": retryCycleAttemptCount(&task),
		"delivery_branch": task.DeliveryBranch, "last_error": task.LastError,
		"created_at": task.CreatedAt, "updated_at": task.UpdatedAt,
	}
}

func taskDetail(task model.Task) gin.H {
	result := taskSummary(task)
	result["objective"] = task.Objective
	result["acceptance_criteria"] = task.AcceptanceCriteria
	result["context_json"] = task.ContextJSON
	result["callback_url"] = task.CallbackURL
	return result
}
