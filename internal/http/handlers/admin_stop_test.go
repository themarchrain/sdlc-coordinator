package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

type fakeAdminStops struct {
	fakeAdminOperations
	actor    orchestrator.StopActor
	id, kind string
	result   *orchestrator.StopResult
	err      error
}

func (f *fakeAdminStops) stop(id, kind string, actor orchestrator.StopActor) (*orchestrator.StopResult, error) {
	f.id, f.kind, f.actor = id, kind, actor
	return f.result, f.err
}
func (f *fakeAdminStops) StopTask(_ context.Context, id string, a orchestrator.StopActor, _ string) (*orchestrator.StopResult, error) {
	return f.stop(id, "task", a)
}
func (f *fakeAdminStops) StopRun(_ context.Context, id string, a orchestrator.StopActor, _ string) (*orchestrator.StopResult, error) {
	return f.stop(id, "run", a)
}
func (f *fakeAdminStops) StopExecution(_ context.Context, id string, a orchestrator.StopActor, _ string) (*orchestrator.StopResult, error) {
	return f.stop(id, "execution", a)
}

func TestAdminStopThreeTargetsAndTrustedActor(t *testing.T) {
	for _, target := range []struct{ path, kind string }{{"/tasks/task-1/stop", "task"}, {"/runs/run-1/stop", "run"}, {"/agents/executions/executions/execution-1/stop", "execution"}} {
		for _, confirmed := range []bool{false, true} {
			t.Run(target.kind+fmt.Sprint(confirmed), func(t *testing.T) {
				status := model.StopDeliveryPending
				if confirmed {
					status = model.StopDeliveryConfirmed
				}
				ops := &fakeAdminStops{result: &orchestrator.StopResult{StopID: "stop-one", TaskStatus: model.TaskStatusInterrupted, DeliveryStatus: status, ConfirmationKind: model.StopConfirmationNoRemoteTarget}}
				router := gin.New()
				router.Use(func(c *gin.Context) { c.Set(cpActorKey, &controlplane.Actor{ID: 17, Username: "admin"}) })
				NewAdminOperationHandler(ops, nil, orchestrator.TickRequest{}).RegisterRoutes(router.Group("/api/admin"))
				response := performAdminOperationRequest(router, http.MethodPost, "/api/admin"+target.path, map[string]string{"reason": "closed"})
				expected := http.StatusAccepted
				if confirmed {
					expected = http.StatusOK
				}
				if response.Code != expected || ops.kind != target.kind || ops.actor.UserID != "17" {
					t.Fatalf("bad stop dispatch: %d %s actor=%+v kind=%s", response.Code, response.Body.String(), ops.actor, ops.kind)
				}
			})
		}
	}
}

func TestAdminStopRejectsUntrustedInputAndState(t *testing.T) {
	for _, test := range []struct {
		name   string
		body   any
		err    error
		status int
	}{
		{"actor", map[string]string{"requested_by": "forged"}, nil, 400},
		{"endpoint", map[string]string{"runner_endpoint": "http://other"}, nil, 400},
		{"long reason", map[string]string{"reason": strings.Repeat("中", 501)}, nil, 400},
		{"old run", map[string]string{}, repository.ErrStopNotCurrentRun, 409},
		{"terminal", map[string]string{}, repository.ErrStopAlreadyTerminal, 409},
		{"missing", map[string]string{}, repository.ErrNotFound, 404},
		{"unavailable", map[string]string{}, orchestrator.ErrStopUnavailable, 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			ops := &fakeAdminStops{err: test.err, result: &orchestrator.StopResult{}}
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set(cpActorKey, &controlplane.Actor{ID: 17}) })
			NewAdminOperationHandler(ops, nil, orchestrator.TickRequest{}).RegisterRoutes(router.Group("/api/admin"))
			response := performAdminOperationRequest(router, http.MethodPost, "/api/admin/tasks/task/stop", test.body)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAdminStopDetailAndTimeline(t *testing.T) {
	router, db := newAdminQueryHandlerTestRouter(t)
	seedAdminQueryTask(t, db)
	if err := store.Migrate(db, &model.StopRequest{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Create(&model.StopRequest{StopID: "stop-detail", DedupeKey: "task:task-admin", TaskID: "task-admin", Scope: model.StopScopeTask, RequestedBy: "17", RequestedAt: now, ReasonCode: "manual_stop", Reason: "closed", DeliveryStatus: model.StopDeliveryConfirmed, ConfirmationKind: model.StopConfirmationNoRemoteTarget, ConfirmedAt: &now, RunnerEndpoint: "private-runner"}).Error; err != nil {
		t.Fatal(err)
	}
	response := performAdminQueryRequest(router, "/api/admin/tasks/task-admin")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"confirmation_kind":"no_remote_target"`) || strings.Contains(response.Body.String(), "private-runner") || strings.Contains(response.Body.String(), `"exit_confirmed":true`) {
		t.Fatalf("invalid stop detail: %d %s", response.Code, response.Body.String())
	}
	timeline := performAdminQueryRequest(router, "/api/admin/tasks/task-admin/timeline")
	if timeline.Code != 200 || !strings.Contains(timeline.Body.String(), "人工停止") || !strings.Contains(timeline.Body.String(), "无需取消执行") {
		t.Fatalf("missing stop timeline: %d %s", timeline.Code, timeline.Body.String())
	}
}

func TestAdminStopUnavailableTenantCannotUseConstructorFallback(t *testing.T) {
	fallback := &fakeAdminStops{result: &orchestrator.StopResult{DeliveryStatus: model.StopDeliveryConfirmed}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(cpActorKey, &controlplane.Actor{ID: 17}) })
	NewAdminOperationHandler(fallback, nil, orchestrator.TickRequest{}, unavailableEngines{}).RegisterRoutes(router.Group("/api/admin"))
	response := performAdminOperationRequest(router, http.MethodPost, "/api/admin/tasks/task/stop", map[string]string{})
	if response.Code != 503 || fallback.id != "" {
		t.Fatalf("tenant stop fell back: %d calls=%s", response.Code, fallback.id)
	}
}
