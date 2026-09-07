package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

func TestListTasksNormalizesPaginationAndAppliesFilters(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	seedTasks(t, db,
		model.Task{
			TaskID:     "task-old",
			DedupeKey:  "old",
			Type:       "develop_from_issue",
			Role:       "DeveloperWorker",
			SourceRepo: "group/repo",
			Status:     model.TaskStatusSucceeded,
		},
		model.Task{
			TaskID:     "task-new",
			DedupeKey:  "new",
			Type:       "develop_from_issue",
			Role:       "DeveloperWorker",
			SourceRepo: "group/repo",
			Status:     model.TaskStatusRunning,
		},
	)

	page, err := service.ListTasks(context.Background(), TaskFilter{
		PageRequest: PageRequest{Page: 0, PageSize: 500},
		Status:      string(model.TaskStatusRunning),
		Role:        "DeveloperWorker",
		Repository:  "group/repo",
	})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if page.Page != 1 || page.PageSize != 100 || page.Total != 1 {
		t.Fatalf("unexpected page: %#v", page)
	}
	if len(page.Items) != 1 || page.Items[0].TaskID != "task-new" {
		t.Fatalf("unexpected items: %#v", page.Items)
	}
}

func TestListRunsFiltersByTaskRoleAndStatus(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	seedRuns(t, db,
		model.Run{
			RunID:      "run-1",
			TaskID:     "task-1",
			WorkerRole: "DeveloperWorker",
			Status:     model.RunStatusRunning,
		},
		model.Run{
			RunID:      "run-2",
			TaskID:     "task-2",
			WorkerRole: "ReviewerWorker",
			Status:     model.RunStatusSucceeded,
		},
	)

	page, err := service.ListRuns(context.Background(), RunFilter{
		TaskID:     "task-2",
		WorkerRole: "ReviewerWorker",
		Status:     string(model.RunStatusSucceeded),
	})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if page.Page != 1 || page.PageSize != 20 || page.Total != 1 {
		t.Fatalf("unexpected page: %#v", page)
	}
	if len(page.Items) != 1 || page.Items[0].RunID != "run-2" {
		t.Fatalf("unexpected items: %#v", page.Items)
	}
}

func TestListTasksAndOverviewFilterByRepositoryID(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	migrateRegisteredRepositories(t, db)
	seedRegisteredRepository(t, db, "repo_a", "group/repo-a", registeredrepo.StatusEnabled)
	seedRegisteredRepository(t, db, "repo_b", "group/repo-b", registeredrepo.StatusDisabled)

	repoA := "repo_a"
	repoB := "repo_b"
	seedTasks(t, db,
		model.Task{
			TaskID:       "task-a",
			DedupeKey:    "task-a",
			Type:         "develop_from_issue",
			Role:         "DeveloperWorker",
			SourceRepo:   "group/repo-a",
			RepositoryID: &repoA,
			Status:       model.TaskStatusRunning,
		},
		model.Task{
			TaskID:       "task-b",
			DedupeKey:    "task-b",
			Type:         "develop_from_issue",
			Role:         "DeveloperWorker",
			SourceRepo:   "group/repo-b",
			RepositoryID: &repoB,
			Status:       model.TaskStatusHumanRequired,
		},
	)

	page, err := service.ListTasks(context.Background(), TaskFilter{RepositoryID: "repo_a"})
	if err != nil {
		t.Fatalf("list tasks repo_a: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].TaskID != "task-a" {
		t.Fatalf("expected only task-a, got %#v", page)
	}

	disabled, err := service.ListTasks(context.Background(), TaskFilter{RepositoryID: "repo_b"})
	if err != nil {
		t.Fatalf("disabled repository history should remain queryable: %v", err)
	}
	if disabled.Total != 1 || disabled.Items[0].TaskID != "task-b" {
		t.Fatalf("expected task-b for disabled repo, got %#v", disabled)
	}

	overview, err := service.Overview(context.Background(), OverviewFilter{RepositoryID: "repo_a"})
	if err != nil {
		t.Fatalf("overview repo_a: %v", err)
	}
	if overview.TasksByStatus[string(model.TaskStatusRunning)] != 1 {
		t.Fatalf("repo_a should count running task, got %#v", overview.TasksByStatus)
	}
	if overview.TasksByStatus[string(model.TaskStatusHumanRequired)] != 0 {
		t.Fatalf("repo_a overview leaked repo_b: %#v", overview.TasksByStatus)
	}

	if _, err := service.ListTasks(context.Background(), TaskFilter{RepositoryID: "missing"}); !errors.Is(err, registeredrepo.ErrNotFound) {
		t.Fatalf("missing repository_id: %v", err)
	}
	if _, err := service.Overview(context.Background(), OverviewFilter{RepositoryID: "missing"}); !errors.Is(err, registeredrepo.ErrNotFound) {
		t.Fatalf("missing overview repository_id: %v", err)
	}
}

func TestListRelatedEntitiesFilterByTaskRepositoryID(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	migrateRegisteredRepositories(t, db)
	seedRegisteredRepository(t, db, "repo_a", "group/repo-a", registeredrepo.StatusEnabled)
	seedRegisteredRepository(t, db, "repo_b", "group/repo-b", registeredrepo.StatusEnabled)

	repoA := "repo_a"
	repoB := "repo_b"
	seedTasks(t, db,
		model.Task{TaskID: "task-a", DedupeKey: "task-a", Type: "develop_from_issue", Role: "DeveloperWorker", RepositoryID: &repoA, Status: model.TaskStatusRunning},
		model.Task{TaskID: "task-b", DedupeKey: "task-b", Type: "develop_from_issue", Role: "DeveloperWorker", RepositoryID: &repoB, Status: model.TaskStatusPending},
	)
	seedRuns(t, db,
		model.Run{RunID: "run-a", TaskID: "task-a", WorkerRole: "DeveloperWorker", Status: model.RunStatusRunning},
		model.Run{RunID: "run-b", TaskID: "task-b", WorkerRole: "DeveloperWorker", Status: model.RunStatusSucceeded},
	)
	if err := db.Create(&model.Writeback{WritebackID: "wb-a", TaskID: "task-a", RunID: "run-a", DedupeKey: "wb-a", Kind: "create_merge_request", ProjectID: "group/repo-a", Status: model.WritebackStatusFailed}).Error; err != nil {
		t.Fatalf("seed writeback: %v", err)
	}
	if err := db.Create(&model.Writeback{WritebackID: "wb-b", TaskID: "task-b", RunID: "run-b", DedupeKey: "wb-b", Kind: "create_merge_request", ProjectID: "group/repo-b", Status: model.WritebackStatusSucceeded}).Error; err != nil {
		t.Fatalf("seed writeback b: %v", err)
	}
	if err := db.Create(&model.Effect{EffectID: "effect-a", TaskID: "task-a", RunID: "run-a", DedupeKey: "effect-a", Kind: "send_im_message", Target: "default", Status: model.EffectStatusFailed}).Error; err != nil {
		t.Fatalf("seed effect: %v", err)
	}
	if err := db.Create(&model.Artifact{ArtifactID: "artifact-a", TaskID: "task-a", RunID: "run-a", Kind: "diff", Name: "a.patch"}).Error; err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	if err := db.Create(&model.Artifact{ArtifactID: "artifact-b", TaskID: "task-b", RunID: "run-b", Kind: "diff", Name: "b.patch"}).Error; err != nil {
		t.Fatalf("seed artifact b: %v", err)
	}
	if err := db.Create(&model.ResultSubmission{SubmissionID: "sub-a", TaskID: "task-a", RunID: "run-a", Source: "mcp", ToolName: "submit_developer_result", Role: "DeveloperWorker", PayloadHash: "h-a", RawPayloadJSON: `{}`, CanonicalJSON: `{}`, Status: model.ResultSubmissionStatusSucceeded}).Error; err != nil {
		t.Fatalf("seed submission: %v", err)
	}
	if err := db.Create(&model.ResultSubmission{SubmissionID: "sub-b", TaskID: "task-b", RunID: "run-b", Source: "mcp", ToolName: "submit_developer_result", Role: "DeveloperWorker", PayloadHash: "h-b", RawPayloadJSON: `{}`, CanonicalJSON: `{}`, Status: model.ResultSubmissionStatusSucceeded}).Error; err != nil {
		t.Fatalf("seed submission b: %v", err)
	}
	if err := db.Create(&model.InboundEvent{EventID: "event-a", DedupeKey: "event-a", Source: "gitlab", EventType: "issue", Status: model.InboundEventStatusTaskCreated, RepositoryID: &repoA}).Error; err != nil {
		t.Fatalf("seed inbound a: %v", err)
	}
	if err := db.Create(&model.InboundEvent{EventID: "event-b", DedupeKey: "event-b", Source: "gitlab", EventType: "issue", Status: model.InboundEventStatusTaskCreated, RepositoryID: &repoB}).Error; err != nil {
		t.Fatalf("seed inbound b: %v", err)
	}
	if err := db.Create(&model.Dependency{DependencyID: "dep-cross", ParentTaskID: "task-a", ChildTaskID: "task-b", Relation: "blocks", Status: model.DependencyStatusPending}).Error; err != nil {
		t.Fatalf("seed dependency: %v", err)
	}

	ctx := context.Background()
	runs, err := service.ListRuns(ctx, RunFilter{RepositoryID: "repo_a"})
	if err != nil || runs.Total != 1 || runs.Items[0].RunID != "run-a" {
		t.Fatalf("runs: %#v %v", runs, err)
	}
	artifacts, err := service.ListArtifacts(ctx, EntityFilter{RepositoryID: "repo_a"})
	if err != nil || artifacts.Total != 1 || artifacts.Items[0].ArtifactID != "artifact-a" {
		t.Fatalf("artifacts: %#v %v", artifacts, err)
	}
	writebacks, err := service.ListWritebacks(ctx, EntityFilter{RepositoryID: "repo_a"})
	if err != nil || writebacks.Total != 1 || writebacks.Items[0].WritebackID != "wb-a" {
		t.Fatalf("writebacks: %#v %v", writebacks, err)
	}
	effects, err := service.ListEffects(ctx, EntityFilter{RepositoryID: "repo_a"})
	if err != nil || effects.Total != 1 || effects.Items[0].EffectID != "effect-a" {
		t.Fatalf("effects: %#v %v", effects, err)
	}
	submissions, err := service.ListResultSubmissions(ctx, EntityFilter{RepositoryID: "repo_a"})
	if err != nil || submissions.Total != 1 || submissions.Items[0].SubmissionID != "sub-a" {
		t.Fatalf("submissions: %#v %v", submissions, err)
	}
	events, err := service.ListInboundEvents(ctx, EntityFilter{RepositoryID: "repo_a"})
	if err != nil || events.Total != 1 || events.Items[0].EventID != "event-a" {
		t.Fatalf("inbound events: %#v %v", events, err)
	}

	dependencies, err := service.ListDependencies(ctx, DependencyFilter{RepositoryID: "repo_a"})
	if err != nil || dependencies.Total != 1 || dependencies.Items[0].DependencyID != "dep-cross" {
		t.Fatalf("dependencies: %#v %v", dependencies, err)
	}
	if got := stringPtrValue(dependencies.Items[0].ParentRepositoryID); got != "repo_a" {
		t.Fatalf("parent repository: %q", got)
	}
	if got := stringPtrValue(dependencies.Items[0].ChildRepositoryID); got != "repo_b" {
		t.Fatalf("child repository: %q", got)
	}

	overview, err := service.Overview(ctx, OverviewFilter{RepositoryID: "repo_a"})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.RunsByStatus[string(model.RunStatusRunning)] != 1 {
		t.Fatalf("repo_a run counts leaked repo_b: %#v", overview.RunsByStatus)
	}
	if overview.WritebacksByStatus[string(model.WritebackStatusFailed)] != 1 {
		t.Fatalf("repo_a writeback counts: %#v", overview.WritebacksByStatus)
	}
	if overview.WritebacksByStatus[string(model.WritebackStatusSucceeded)] != 0 {
		t.Fatalf("repo_a overview leaked repo_b writeback: %#v", overview.WritebacksByStatus)
	}
}

func TestGetTaskAndRunReturnRepositoryNotFound(t *testing.T) {
	service, _ := newQueryServiceTestDB(t)

	if _, err := service.GetTask(context.Background(), "missing"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("unexpected task error: %v", err)
	}
	if _, err := service.GetRun(context.Background(), "missing"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("unexpected run error: %v", err)
	}
}

func TestQueryServiceListsAndGetsEveryPersistedEntity(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	seedAllAdminEntities(t, db)
	ctx := context.Background()

	artifacts, err := service.ListArtifacts(ctx, EntityFilter{TaskID: "task-1"})
	if err != nil || artifacts.Total != 1 || artifacts.Items[0].ArtifactID != "artifact-1" {
		t.Fatalf("artifacts: %#v %v", artifacts, err)
	}
	if artifact, err := service.GetArtifact(ctx, "artifact-1"); err != nil || artifact.ArtifactID != "artifact-1" {
		t.Fatalf("get artifact: %#v %v", artifact, err)
	}

	dependencies, err := service.ListDependencies(ctx, DependencyFilter{ParentTaskID: "task-1"})
	if err != nil || dependencies.Total != 1 || dependencies.Items[0].DependencyID != "dependency-1" {
		t.Fatalf("dependencies: %#v %v", dependencies, err)
	}
	if dependency, err := service.GetDependency(ctx, "dependency-1"); err != nil || dependency.DependencyID != "dependency-1" {
		t.Fatalf("get dependency: %#v %v", dependency, err)
	}

	writebacks, err := service.ListWritebacks(ctx, EntityFilter{Status: "succeeded"})
	if err != nil || writebacks.Total != 1 || writebacks.Items[0].WritebackID != "writeback-1" {
		t.Fatalf("writebacks: %#v %v", writebacks, err)
	}
	if writeback, err := service.GetWriteback(ctx, "writeback-1"); err != nil || writeback.WritebackID != "writeback-1" {
		t.Fatalf("get writeback: %#v %v", writeback, err)
	}

	effects, err := service.ListEffects(ctx, EntityFilter{Kind: "send_im_message"})
	if err != nil || effects.Total != 1 || effects.Items[0].EffectID != "effect-1" {
		t.Fatalf("effects: %#v %v", effects, err)
	}
	if effect, err := service.GetEffect(ctx, "effect-1"); err != nil || effect.EffectID != "effect-1" {
		t.Fatalf("get effect: %#v %v", effect, err)
	}

	events, err := service.ListInboundEvents(ctx, EntityFilter{Source: "gitlab"})
	if err != nil || events.Total != 1 || events.Items[0].EventID != "event-1" {
		t.Fatalf("inbound events: %#v %v", events, err)
	}
	if event, err := service.GetInboundEvent(ctx, "event-1"); err != nil || event.EventID != "event-1" {
		t.Fatalf("get inbound event: %#v %v", event, err)
	}

	submissions, err := service.ListResultSubmissions(ctx, EntityFilter{RunID: "run-1"})
	if err != nil || submissions.Total != 1 || submissions.Items[0].SubmissionID != "submission-1" {
		t.Fatalf("result submissions: %#v %v", submissions, err)
	}
	if submission, err := service.GetResultSubmission(ctx, "submission-1"); err != nil || submission.SubmissionID != "submission-1" {
		t.Fatalf("get result submission: %#v %v", submission, err)
	}
}

func newQueryServiceTestDB(t *testing.T) (*QueryService, *gorm.DB) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	db, err := store.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dsnName))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(
		db,
		&model.Task{},
		&model.Run{},
		&model.Artifact{},
		&model.Dependency{},
		&model.Writeback{},
		&model.Effect{},
		&model.InboundEvent{},
		&model.ResultSubmission{},
		&model.ResultSubmissionRejection{},
	); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	return NewQueryService(db), db
}

func migrateRegisteredRepositories(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := registeredrepo.NewStore(db).Migrate(); err != nil {
		t.Fatalf("migrate registered repositories: %v", err)
	}
}

func seedRegisteredRepository(t *testing.T, db *gorm.DB, repositoryID, path string, status registeredrepo.RepositoryStatus) {
	t.Helper()
	repo := &registeredrepo.RegisteredRepository{
		RepositoryID:      repositoryID,
		DisplayName:       repositoryID,
		PathWithNamespace: path,
		Status:            status,
	}
	if err := db.Create(repo).Error; err != nil {
		t.Fatalf("seed registered repository %s: %v", repositoryID, err)
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func seedTasks(t *testing.T, db *gorm.DB, tasks ...model.Task) {
	t.Helper()
	for i := range tasks {
		if err := db.Create(&tasks[i]).Error; err != nil {
			t.Fatalf("seed task %s: %v", tasks[i].TaskID, err)
		}
	}
}

func seedRuns(t *testing.T, db *gorm.DB, runs ...model.Run) {
	t.Helper()
	for i := range runs {
		if err := db.Create(&runs[i]).Error; err != nil {
			t.Fatalf("seed run %s: %v", runs[i].RunID, err)
		}
	}
}

func seedAllAdminEntities(t *testing.T, db *gorm.DB) {
	t.Helper()

	entities := []any{
		&model.Artifact{
			ArtifactID: "artifact-1",
			TaskID:     "task-1",
			RunID:      "run-1",
			Kind:       "diff",
			Name:       "change.patch",
		},
		&model.Dependency{
			DependencyID: "dependency-1",
			ParentTaskID: "task-1",
			ChildTaskID:  "task-2",
			Relation:     "upstream_fix",
			Status:       model.DependencyStatusResolved,
		},
		&model.Writeback{
			WritebackID: "writeback-1",
			TaskID:      "task-1",
			RunID:       "run-1",
			DedupeKey:   "writeback-dedupe-1",
			Kind:        "create_merge_request",
			ProjectID:   "group/repo",
			Status:      model.WritebackStatusSucceeded,
		},
		&model.Effect{
			EffectID:  "effect-1",
			TaskID:    "task-1",
			RunID:     "run-1",
			DedupeKey: "effect-dedupe-1",
			Kind:      "send_im_message",
			Target:    "default",
			Status:    model.EffectStatusSucceeded,
		},
		&model.InboundEvent{
			EventID:   "event-1",
			DedupeKey: "event-dedupe-1",
			Source:    "gitlab",
			EventType: "issue",
			Status:    model.InboundEventStatusTaskCreated,
		},
		&model.ResultSubmission{
			SubmissionID:   "submission-1",
			TaskID:         "task-1",
			RunID:          "run-1",
			Source:         "mcp",
			ToolName:       "submit_developer_result",
			Role:           "DeveloperWorker",
			PayloadHash:    "hash-1",
			RawPayloadJSON: `{}`,
			CanonicalJSON:  `{}`,
			Status:         model.ResultSubmissionStatusSucceeded,
		},
	}

	for _, entity := range entities {
		if err := db.Create(entity).Error; err != nil {
			t.Fatalf("seed admin entity %T: %v", entity, err)
		}
	}
}
