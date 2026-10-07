package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/workspaceproto"

	"github.com/gin-gonic/gin"
)

type stubAuthorizationService struct {
	decision workspaceproto.AuthorizationDecision
	err      error
	last     workspaceproto.AuthorizationRequest
	calls    int
}

func (s *stubAuthorizationService) AuthorizeRunnerWorkspaceOperation(_ context.Context, request workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
	s.calls++
	s.last = request
	if s.err != nil {
		return workspaceproto.AuthorizationDecision{}, s.err
	}
	return s.decision, nil
}

func newRunnerAuthorizationRouter(service RunnerAuthorizationService, token string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/internal/runner-authorization", NewRunnerAuthorizationHandler(service, token).Handle)
	return router
}

func postAuthorization(router *gin.Engine, token string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, "/internal/runner-authorization", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// TestRunnerAuthorizationHandlerFailClosed 通道未配置 503、令牌不符 401、
// 内部错误 503：任何不完全确认都不得被 Runner 当作授权通过。
func TestRunnerAuthorizationHandlerFailClosed(t *testing.T) {
	stub := &stubAuthorizationService{decision: workspaceproto.AuthorizationDecision{Authorized: true}}
	request := workspaceproto.AuthorizationRequest{OperationID: "op-1", Kind: workspaceproto.KindDeveloperPush, TaskID: "task-1", RunID: "run-1"}

	closed := newRunnerAuthorizationRouter(stub, "")
	if response := postAuthorization(closed, "any", request); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured channel status = %d, want 503", response.Code)
	}

	router := newRunnerAuthorizationRouter(stub, "deploy-token")
	if response := postAuthorization(router, "wrong-token", request); response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d, want 401", response.Code)
	}
	if response := postAuthorization(router, "", request); response.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want 401", response.Code)
	}

	failing := &stubAuthorizationService{err: context.DeadlineExceeded}
	if response := postAuthorization(newRunnerAuthorizationRouter(failing, "deploy-token"), "deploy-token", request); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("internal error status = %d, want 503", response.Code)
	}
}

// TestRunnerAuthorizationHandlerPassesDecision 合法令牌下透传裁决结果。
func TestRunnerAuthorizationHandlerPassesDecision(t *testing.T) {
	stub := &stubAuthorizationService{decision: workspaceproto.AuthorizationDecision{Authorized: false, Reason: "run is no longer the current run of the task"}}
	router := newRunnerAuthorizationRouter(stub, "deploy-token")
	response := postAuthorization(router, "deploy-token", workspaceproto.AuthorizationRequest{
		OperationID: "op-2", Kind: workspaceproto.KindDeveloperCommit, TenantID: 3, TaskID: "task-2", RunID: "run-2",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var decision workspaceproto.AuthorizationDecision
	if err := json.Unmarshal(response.Body.Bytes(), &decision); err != nil {
		t.Fatalf("decode decision: %v", err)
	}
	if decision.Authorized || decision.Reason == "" {
		t.Fatalf("decision = %+v", decision)
	}
	if stub.calls != 1 || stub.last.TenantID != 3 || stub.last.Kind != workspaceproto.KindDeveloperCommit {
		t.Fatalf("service call = %+v", stub.last)
	}
}
