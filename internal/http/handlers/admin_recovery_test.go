package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/store/repository"

	"github.com/gin-gonic/gin"
)

type fakeAdminRecovery struct {
	assessment orchestrator.RecoveryAssessment
	response   *orchestrator.RecoverTaskResponse
	err        error
	actorID    string
	request    orchestrator.RecoverTaskRequest
}

func (f *fakeAdminRecovery) Tick(context.Context, orchestrator.TickRequest) (*orchestrator.TickResult, error) {
	return nil, nil
}
func (f *fakeAdminRecovery) RetryEffect(context.Context, string) (*model.Effect, error) {
	return nil, nil
}
func (f *fakeAdminRecovery) RebindTask(context.Context, string, string) (*model.Task, error) {
	return nil, nil
}
func (f *fakeAdminRecovery) AssessTaskRecovery(context.Context, string) (orchestrator.RecoveryAssessment, error) {
	return f.assessment, f.err
}
func (f *fakeAdminRecovery) RecoverBlockedTask(_ context.Context, _ string, actorID string, request orchestrator.RecoverTaskRequest) (*orchestrator.RecoverTaskResponse, error) {
	f.actorID = actorID
	f.request = request
	return f.response, f.err
}

func TestRecoveryHandlerGetAndPost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	operations := &fakeAdminRecovery{
		assessment: orchestrator.RecoveryAssessment{
			Classification: "transient", Eligible: true, ReasonCode: "eligible", ReasonSummary: "可恢复",
			BlockRevision: "revision-1", SafetyBlockers: []orchestrator.SafetyBlocker{},
		},
		response: &orchestrator.RecoverTaskResponse{Recovery: &model.TaskRecovery{RecoveryID: "recovery-1"}, Task: &model.Task{TaskID: "task-1", Status: model.TaskStatusPending}},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(cpActorKey, &controlplane.Actor{ID: 17}) })
	NewAdminOperationHandler(operations, nil, orchestrator.TickRequest{}).RegisterRoutes(router.Group("/api/admin"))

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/admin/tasks/task-1/recovery", nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"eligible":true`) || strings.Contains(get.Body.String(), "revision-1") == false {
		t.Fatalf("GET recovery = %d %s", get.Code, get.Body.String())
	}

	post := httptest.NewRecorder()
	router.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/admin/tasks/task-1/recovery", strings.NewReader(`{"request_id":"req-1","block_revision":"revision-1","reason":"retry"}`)))
	if post.Code != http.StatusOK || operations.actorID != "17" || operations.request.RequestID != "req-1" {
		t.Fatalf("POST recovery = %d actor=%q request=%+v body=%s", post.Code, operations.actorID, operations.request, post.Body.String())
	}
}

func TestRecoveryHandlerValidatesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	operations := &fakeAdminRecovery{err: repository.ErrRecoveryRiskRequired}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(cpActorKey, &controlplane.Actor{ID: 17}) })
	NewAdminOperationHandler(operations, nil, orchestrator.TickRequest{}).RegisterRoutes(router.Group("/api/admin"))

	for name, body := range map[string]string{
		"missing ack": `{"request_id":"req-1","block_revision":"revision-1","reason":"retry"}`,
		"too long":    `{"request_id":"req-1","block_revision":"revision-1","reason":"` + strings.Repeat("a", 1001) + `"}`,
		"oversized":   `{"request_id":"req-1","block_revision":"revision-1","reason":"` + strings.Repeat("a", 9000) + `"}`,
		"unknown":     `{"request_id":"req-1","block_revision":"revision-1","force":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/tasks/task-1/recovery", strings.NewReader(body)))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestRecoveryHandlerUnavailableTenantDoesNotFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ops := &fakeAdminRecovery{}
	NewAdminOperationHandler(ops, nil, orchestrator.TickRequest{}, unavailableEngines{}).RegisterRoutes(router.Group("/api/admin"))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/tasks/task-1/recovery", nil))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "operations_unavailable") {
		t.Fatalf("unavailable recovery = %d %s", recorder.Code, recorder.Body.String())
	}
}
