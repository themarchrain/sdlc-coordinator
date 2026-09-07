package orchestrator

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
)

// fixtureRepositoryID 是写回/派发测试默认的注册仓绑定 ID。
const fixtureRepositoryID = "repo-framework"

// backendResolverFor 把固定后端按 repositoryID 绑成测试用 resolver。
func backendResolverFor(repositoryID string, backend codehost.Backend) CodeHostBackendResolver {
	return &staticCodeHostResolver{backends: map[string]codehost.Backend{repositoryID: backend}}
}

// stubRunnerCredential 返回固定 git 凭据的解析闭包,供派发链测试注入 settings。
func stubRunnerCredential() func(context.Context, *model.Task) (*workspace.GitCredential, error) {
	return func(context.Context, *model.Task) (*workspace.GitCredential, error) {
		return &workspace.GitCredential{Username: "oauth2", Token: "git-token"}, nil
	}
}

type staticCodeHostResolver struct {
	backends       map[string]codehost.Backend
	allowedProject map[string]string
	err            error
	calls          int
}

func (resolver *staticCodeHostResolver) Resolve(_ context.Context, task *model.Task, writeback *model.Writeback) (codehost.Backend, error) {
	resolver.calls++
	if resolver.err != nil {
		return nil, resolver.err
	}
	if task == nil || task.RepositoryID == nil {
		return nil, errors.New("task repository_id is required")
	}
	allowed := resolver.allowedProject[*task.RepositoryID]
	if allowed != "" && writeback.ProjectID != allowed {
		return nil, errors.New("writeback project does not belong to repository")
	}
	backend := resolver.backends[*task.RepositoryID]
	if backend == nil {
		return nil, errors.New("no backend for repository")
	}
	return backend, nil
}

type recordingProjectProbe struct {
	calls int
	err   error
	token string
}

func (probe *recordingProjectProbe) GetProject(_ context.Context, _, token, path string) (registeredrepo.ProjectIdentity, error) {
	probe.calls++
	probe.token = token
	if probe.err != nil {
		return registeredrepo.ProjectIdentity{}, probe.err
	}
	return registeredrepo.ProjectIdentity{ID: "1", PathWithNamespace: path}, nil
}

func newWritebackAuthHarness(t *testing.T, writebackErr error, probe *recordingProjectProbe) writebackAuthHarness {
	t.Helper()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.Writeback{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repoStore := registeredrepo.NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate repos: %v", err)
	}
	repo := &registeredrepo.RegisteredRepository{
		RepositoryID:      "repo_a",
		DisplayName:       "A",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com",
		CodeHostProjectID: "1",
		PathWithNamespace: "group/a",
		Token:             "token-a",
		Status:            registeredrepo.StatusEnabled,
	}
	if err := repoStore.Create(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if probe == nil {
		probe = &recordingProjectProbe{}
	}
	backend := &fakeGitLabWritebackBackend{err: writebackErr}
	sibling := &fakeGitLabWritebackBackend{}
	resolver := &staticCodeHostResolver{
		backends:       map[string]codehost.Backend{"repo_a": backend, "repo_b": sibling},
		allowedProject: map[string]string{"repo_a": "1", "repo_b": "2"},
	}
	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	writebackRepo := repository.NewWritebackRepository(db)
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(repository.NewArtifactRepository(db)),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(resolver),
		WithRepositoryLookup(repoStore),
		WithWritebackAuthHandler(registeredrepo.NewWritebackAuthHandler(repoStore, nil).
			WithProbeFactory(func(string) registeredrepo.ProjectProbe { return probe })),
	)
	return writebackAuthHarness{
		orch:       orch,
		tasks:      taskRepo,
		runs:       runRepo,
		writebacks: writebackRepo,
		repos:      repoStore,
		backend:    backend,
		sibling:    sibling,
		repoID:     repo.RepositoryID,
	}
}

type writebackAuthHarness struct {
	orch       *Orchestrator
	tasks      *repository.TaskRepository
	runs       *repository.RunRepository
	writebacks *repository.WritebackRepository
	repos      *registeredrepo.Store
	backend    *fakeGitLabWritebackBackend
	sibling    *fakeGitLabWritebackBackend
	repoID     string
}

func (h writebackAuthHarness) bindTask(t *testing.T, taskID, runID string) {
	t.Helper()
	ctx := context.Background()
	createGitLabWritebackFixture(t, ctx, h.tasks, h.runs, taskID, runID)
	task, err := h.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	task.RepositoryID = &h.repoID
	if err := h.tasks.Update(ctx, task); err != nil {
		t.Fatal(err)
	}
}
