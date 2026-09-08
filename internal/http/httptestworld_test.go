package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/connectors"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/events"
	"sdlc-coordinator/internal/http/handlers"
	"sdlc-coordinator/internal/model"
	agentobservability "sdlc-coordinator/internal/observability/agent"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/people"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/requirements"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/tenantdb"

	"github.com/gin-gonic/gin"
)

const (
	testPortalPassword = "test-member-password"
	testSuperPassword  = "test-super-password"
)

// fullyWiredWorld 是生产形态 gin 装配的测试夹具：控制面门户会话门禁
// /api/admin/*，业务服务挂同一测试库。
type fullyWiredWorld struct {
	Router          *gin.Engine
	ControlPlane    *controlplane.Service
	TenantStores    tenantdb.Resolver
	defaultTenantID uint
	superActor      *controlplane.Actor
}

func newFullyWiredWorld(t *testing.T) *fullyWiredWorld {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := openHTTPTestDB(t)
	if err := store.Migrate(db,
		&model.Task{}, &model.Run{}, &model.Artifact{}, &model.Dependency{},
		&model.Writeback{}, &model.Effect{}, &model.InboundEvent{},
		&model.ResultSubmission{}, &model.ResultSubmissionRejection{},
		&model.ConnectorInstance{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	peopleRepository := people.NewRepository(db)
	if err := peopleRepository.Migrate(); err != nil {
		t.Fatalf("migrate people: %v", err)
	}
	repoStore := registeredrepo.NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate repositories: %v", err)
	}
	requirementStore := requirements.NewStore(db)
	if err := requirementStore.Migrate(); err != nil {
		t.Fatalf("migrate requirements: %v", err)
	}
	controlPlaneRepository := controlplane.NewRepository(db)
	if err := controlPlaneRepository.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}

	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	inboundRepo := repository.NewInboundEventRepository(db)
	orch := orchestrator.New(taskRepo, runRepo,
		orchestrator.WithResultSubmissionRepository(repository.NewResultSubmissionRepository(db)),
	)
	processor := events.NewProcessor(inboundRepo, orch)
	processor.SetRepositoryLookup(repoStore)

	mcpHandler := handlers.NewMCPHandler(nil, []byte("matrix-test-secret"), taskRepo, runRepo,
		repository.NewResultRejectionRepository(db), 1)

	configurationService := configuration.NewService(nil, configuration.Resolution{Mode: configuration.ModeReady})
	controlPlane := controlplane.NewService(controlPlaneRepository)
	tenantStores := tenantdb.NewSharedMainResolver(db, 0)
	router := NewRouter(orch, processor,
		WithWebhookSecretLookup(repoStore),
		WithAdminQueryService(admin.NewQueryService(tenantStores.Main().DB)),
		WithAgentObservability(agentobservability.NewQuery(tenantStores.Main().DB)),
		WithConfigurationService(configurationService, configuration.NewShutdownSignal()),
		WithPeopleService(people.NewService(peopleRepository), nil),
		WithRepositoryService(registeredrepo.NewService(repoStore, nil, nil)),
		WithRequirementsService(requirements.NewService(requirementStore, repoStore, requirements.NewProviderClientFactory(nil))),
		WithConnectorService(connectors.NewService(connectors.NewConnectorRepository(tenantStores.Main().DB), connectors.NewHTTPClient())),
		WithControlPlaneService(controlPlane),
		WithTenantStoreResolver(tenantStores),
		WithMCPHandler(mcpHandler),
		WithRuntimeMode(configuration.ModeReady),
	)
	return &fullyWiredWorld{
		Router:       router,
		ControlPlane: controlPlane,
		TenantStores: tenantStores,
	}
}

func (world *fullyWiredWorld) seedTenant(t *testing.T, slug string) uint {
	t.Helper()
	actor := world.ensureSuperActor(t)
	tenant, err := world.ControlPlane.CreateTenant(context.Background(), actor, controlplane.TenantInput{
		Slug: slug, DisplayName: slug + " 组织",
	})
	if err != nil {
		t.Fatalf("seed tenant %s: %v", slug, err)
	}
	return tenant.ID
}

func (world *fullyWiredWorld) defaultTenantStore(t *testing.T) *tenantdb.Store {
	t.Helper()
	if world.defaultTenantID == 0 {
		world.defaultTenantID = world.seedTenant(t, "tenant-one")
	}
	store, err := world.TenantStores.Handle(context.Background(), world.defaultTenantID)
	if err != nil {
		t.Fatalf("resolve tenant-one store: %v", err)
	}
	return store
}

func (world *fullyWiredWorld) ensureSuperActor(t *testing.T) *controlplane.Actor {
	t.Helper()
	if world.superActor != nil {
		return world.superActor
	}
	id, err := world.ControlPlane.CreateSuperAdminUser(context.Background(), "t2root", testSuperPassword)
	if err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	world.superActor = &controlplane.Actor{ID: id, Username: "t2root"}
	return world.superActor
}

func (world *fullyWiredWorld) loginSuperAdmin(t *testing.T) string {
	t.Helper()
	world.ensureSuperActor(t)
	result, err := world.ControlPlane.Login(context.Background(), "t2root", testSuperPassword, "10.0.0.2")
	if err != nil {
		t.Fatalf("super admin login: %v", err)
	}
	return result.Token
}

func (world *fullyWiredWorld) seedUserWithRole(t *testing.T, tenantID uint, username, role string) string {
	t.Helper()
	ctx := context.Background()
	user, err := world.ControlPlane.CreateUser(ctx, username, testPortalPassword)
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if _, err := world.ControlPlane.AddMembership(ctx, world.ensureSuperActor(t), tenantID, user.ID, role); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	result, err := world.ControlPlane.Login(ctx, username, testPortalPassword, "10.0.0.1")
	if err != nil {
		t.Fatalf("login %s: %v", username, err)
	}
	return result.Token
}

func (world *fullyWiredWorld) do(t *testing.T, method, path, cookieName, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	}
	recorder := httptest.NewRecorder()
	world.Router.ServeHTTP(recorder, request)
	return recorder
}

func (world *fullyWiredWorld) portalDo(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return world.do(t, method, path, handlers.PortalSessionCookieName, token, body)
}

func decodeJSONMap(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
	return payload
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	payload := decodeJSONMap(t, recorder)
	errObj, _ := payload["error"].(map[string]any)
	code, _ := errObj["code"].(string)
	return code
}

func assertJSONLacksKeys(t *testing.T, value any, forbidden ...string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range forbidden {
			if _, exists := typed[key]; exists {
				t.Fatalf("response must not contain key %q: %#v", key, typed)
			}
		}
		for _, nested := range typed {
			assertJSONLacksKeys(t, nested, forbidden...)
		}
	case []any:
		for _, nested := range typed {
			assertJSONLacksKeys(t, nested, forbidden...)
		}
	}
}
