package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/store/repository"

	"github.com/gin-gonic/gin"
)

func TestAdminOperationDefaultsAndTickDelegateToOrchestrator(t *testing.T) {
	defaults := orchestrator.TickRequest{DispatchLimit: 2, EffectLimit: 5}
	operations := &fakeAdminOperations{tickResult: &orchestrator.TickResult{
		DispatchedTasks: []orchestrator.DispatchTickResult{{TaskID: "task-1", RunID: "run-1", Status: model.TaskStatusRunning}},
	}}
	router := adminOperationRouter(operations, &fakeEffectLookup{}, defaults)

	defaultResponse := performAdminOperationRequest(router, http.MethodGet, "/api/admin/scheduler/defaults", nil)
	if defaultResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultResponse.Code, defaultResponse.Body.String())
	}
	var gotDefaults orchestrator.TickRequest
	if err := json.Unmarshal(defaultResponse.Body.Bytes(), &gotDefaults); err != nil {
		t.Fatal(err)
	}
	if gotDefaults.DispatchLimit != 2 || gotDefaults.EffectLimit != 5 {
		t.Fatalf("defaults=%+v", gotDefaults)
	}

	tickResponse := performAdminOperationRequest(router, http.MethodPost, "/api/admin/scheduler/tick", defaults)
	if tickResponse.Code != http.StatusOK || operations.tickCalls != 1 {
		t.Fatalf("tick status=%d calls=%d body=%s", tickResponse.Code, operations.tickCalls, tickResponse.Body.String())
	}
	if operations.lastTick.EffectLimit != 5 {
		t.Fatalf("tick request=%+v", operations.lastTick)
	}
}

func TestAdminOperationRetryResolvesEffectIDServerSide(t *testing.T) {
	effect := &model.Effect{EffectID: "effect-1", DedupeKey: "im:task:run", Status: model.EffectStatusFailed}
	operations := &fakeAdminOperations{retryResult: &model.Effect{
		EffectID: effect.EffectID, DedupeKey: effect.DedupeKey, Status: model.EffectStatusPending,
	}}
	router := adminOperationRouter(operations, &fakeEffectLookup{effect: effect}, orchestrator.TickRequest{})

	response := performAdminOperationRequest(router, http.MethodPost, "/api/admin/effects/effect-1/retry", nil)
	if response.Code != http.StatusAccepted || operations.retryCalls != 1 || operations.lastDedupeKey != effect.DedupeKey {
		t.Fatalf("status=%d calls=%d key=%q body=%s", response.Code, operations.retryCalls, operations.lastDedupeKey, response.Body.String())
	}
}

func TestAdminOperationRetryMapsLookupAndStateErrors(t *testing.T) {
	tests := []struct {
		name       string
		lookup     *fakeEffectLookup
		operations *fakeAdminOperations
		wantStatus int
		wantCode   string
	}{
		{
			name: "effect missing", lookup: &fakeEffectLookup{err: repository.ErrNotFound}, operations: &fakeAdminOperations{},
			wantStatus: http.StatusNotFound, wantCode: "not_found",
		},
		{
			name: "effect not retryable", lookup: &fakeEffectLookup{effect: &model.Effect{EffectID: "effect-1", DedupeKey: "key"}},
			operations: &fakeAdminOperations{retryErr: orchestrator.ErrEffectNotRetryable},
			wantStatus: http.StatusConflict, wantCode: "effect_not_retryable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := adminOperationRouter(test.operations, test.lookup, orchestrator.TickRequest{})
			response := performAdminOperationRequest(router, http.MethodPost, "/api/admin/effects/effect-1/retry", nil)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != test.wantCode {
				t.Fatalf("code=%q body=%s", body.Error.Code, response.Body.String())
			}
		})
	}
}

type fakeAdminOperations struct {
	tickResult             *orchestrator.TickResult
	tickErr                error
	tickCalls              int
	lastTick               orchestrator.TickRequest
	retryResult            *model.Effect
	retryErr               error
	retryCalls             int
	lastDedupeKey          string
	rebindResult           *model.Task
	rebindErr              error
	rebindCalls            int
	lastRebindTaskID       string
	lastRebindRepositoryID string
}

func (operations *fakeAdminOperations) Tick(_ context.Context, request orchestrator.TickRequest) (*orchestrator.TickResult, error) {
	operations.tickCalls++
	operations.lastTick = request
	return operations.tickResult, operations.tickErr
}

func (operations *fakeAdminOperations) RetryEffect(_ context.Context, dedupeKey string) (*model.Effect, error) {
	operations.retryCalls++
	operations.lastDedupeKey = dedupeKey
	return operations.retryResult, operations.retryErr
}

func (operations *fakeAdminOperations) RebindTask(_ context.Context, taskID, repositoryID string) (*model.Task, error) {
	operations.rebindCalls++
	operations.lastRebindTaskID = taskID
	operations.lastRebindRepositoryID = repositoryID
	return operations.rebindResult, operations.rebindErr
}

type fakeEffectLookup struct {
	effect *model.Effect
	err    error
}

func (lookup *fakeEffectLookup) GetEffect(context.Context, string) (*model.Effect, error) {
	return lookup.effect, lookup.err
}

func adminOperationRouter(operations AdminOperations, lookup AdminEffectLookup, defaults orchestrator.TickRequest) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewAdminOperationHandler(operations, lookup, defaults).RegisterRoutes(router.Group("/api/admin"))
	return router
}

func performAdminOperationRequest(router http.Handler, method, path string, payload any) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(response, request)
	return response
}

func TestAdminOperationRebindRoutesToOperations(t *testing.T) {
	operations := &fakeAdminOperations{rebindResult: &model.Task{TaskID: "task-1", Status: model.TaskStatusPending}}
	router := adminOperationRouter(operations, &fakeEffectLookup{}, orchestrator.TickRequest{})

	response := performAdminOperationRequest(router, http.MethodPost, "/api/admin/tasks/task-1/rebind", map[string]string{"repository_id": "repo-1"})
	if response.Code != http.StatusOK {
		t.Fatalf("rebind status=%d body=%s", response.Code, response.Body.String())
	}
	if operations.rebindCalls != 1 || operations.lastRebindTaskID != "task-1" || operations.lastRebindRepositoryID != "repo-1" {
		t.Fatalf("unexpected rebind delegation: %#v", operations)
	}
}
