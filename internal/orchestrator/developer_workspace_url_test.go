package orchestrator

import (
	"context"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
)

// 空值安全承诺:resolver 返回 (nil, nil) 的装配形态必须得到明确错误而非 panic。
func TestRegisteredRepositoryForRejectsNilRepositoryRow(t *testing.T) {
	settings := ReviewerRunnerSettings{ResolveRepository: func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
		return nil, nil
	}}
	if _, err := settings.RegisteredRepositoryFor(context.Background(), &model.Task{TaskID: "task-nil-row"}); err == nil {
		t.Fatal("nil repository row should be rejected with an explicit error")
	}
}

// 注册仓行是 workspace URL 唯一权威源:事件载荷 URL 受平台 ROOT_URL 支配,
// 可能指向协调器不可达的地址,派发时必须按仓行重建。
func TestBuildDeveloperWorkspaceRequestDerivesRepositoryURLFromRegisteredRow(t *testing.T) {
	task := &model.Task{
		TaskID: "task-developer-url", Role: "DeveloperWorker", Type: "develop_from_issue", TargetRepo: "group/project",
		ContextJSON: `{"repos":[{"name":"project","url":"https://gitlab.example.com/group/project.git","branch":"master","access":"writable"}]}`,
	}
	run := &model.Run{RunID: "run-developer-url", TaskID: task.TaskID, WorkerRole: "DeveloperWorker"}
	request, err := buildDeveloperWorkspaceRequest(context.Background(), task, run,
		ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()})
	if err != nil {
		t.Fatalf("build developer workspace request: %v", err)
	}
	if request.RepositoryURL != "https://gitlab.example/group/project.git" {
		t.Fatalf("workspace url did not derive from registered row: %q", request.RepositoryURL)
	}
	if request.Credential == nil || request.Credential.Token != "git-token" {
		t.Fatalf("credential did not derive from registered row: %#v", request.Credential)
	}
}

func TestBuildReviewerWorkspaceRequestDerivesRepositoryURLFromRegisteredRow(t *testing.T) {
	task := &model.Task{
		TaskID: "task-reviewer-url", Role: "ReviewerWorker", Type: "review_mr", TargetRepo: "group/project",
		ContextJSON: `{"merge_request":{"source_branch":"feature/x","target_branch":"master","head_sha":"a","base_sha":"b","review_mode":"full"},"repos":[{"name":"project","url":"https://gitlab.example.com/group/project.git","branch":"feature/x","access":"readable"}]}`,
	}
	run := &model.Run{RunID: "run-reviewer-url", TaskID: task.TaskID, WorkerRole: "ReviewerWorker"}
	request, err := buildReviewerWorkspaceRequest(context.Background(), task, run,
		ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()})
	if err != nil {
		t.Fatalf("build reviewer workspace request: %v", err)
	}
	if request.RepositoryURL != "https://gitlab.example/group/project.git" {
		t.Fatalf("workspace url did not derive from registered row: %q", request.RepositoryURL)
	}
	if request.Credential == nil || request.Credential.Token != "git-token" {
		t.Fatalf("credential did not derive from registered row: %#v", request.Credential)
	}
}

// rework 链:ResumeRework 回填的 context_json 仍携带事件 URL,派发必须同样按仓行取值。
func TestBuildDeveloperWorkspaceRequestReworkIgnoresEventRepositoryURL(t *testing.T) {
	task := &model.Task{
		TaskID: "task-rework-url", Role: "DeveloperWorker", Type: model.TaskTypeReworkMergeRequest, TargetRepo: "group/project",
		DeliveryBranch: "feature/existing",
		ContextJSON:    `{"merge_request":{"source_branch":"feature/existing","target_branch":"master","expected_head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"repos":[{"name":"project","url":"https://gitlab.example.com/group/project.git","branch":"feature/existing","access":"writable"}]}`,
	}
	run := &model.Run{RunID: "run-rework-url", TaskID: task.TaskID, WorkerRole: "DeveloperWorker"}
	request, err := buildDeveloperWorkspaceRequest(context.Background(), task, run,
		ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()})
	if err != nil {
		t.Fatalf("build rework workspace request: %v", err)
	}
	if request.RepositoryURL != "https://gitlab.example/group/project.git" {
		t.Fatalf("rework workspace url did not derive from registered row: %q", request.RepositoryURL)
	}
}

// 仓行身份字段缺失(base_url/path 为空)时不得派发出半成品 URL,必须显式失败。
func TestBuildWorkspaceRequestsRejectRowWithoutIdentityFields(t *testing.T) {
	emptyRow := func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
		return &registeredrepo.RegisteredRepository{RepositoryID: "repo-empty-row", Provider: "gitlab"}, nil
	}
	developerTask := &model.Task{
		TaskID: "task-empty-row-dev", Role: "DeveloperWorker", Type: "develop_from_issue", TargetRepo: "group/project",
		ContextJSON: `{"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"master","access":"writable"}]}`,
	}
	if _, err := buildDeveloperWorkspaceRequest(context.Background(), developerTask,
		&model.Run{RunID: "run-empty-row-dev", TaskID: developerTask.TaskID, WorkerRole: "DeveloperWorker"},
		ReviewerRunnerSettings{ResolveRepository: emptyRow}); err == nil || !strings.Contains(err.Error(), "cannot derive clone url") {
		t.Fatalf("developer dispatch should reject row without identity fields, got %v", err)
	}
	reviewerTask := &model.Task{
		TaskID: "task-empty-row-rev", Role: "ReviewerWorker", Type: "review_mr", TargetRepo: "group/project",
		ContextJSON: `{"merge_request":{"source_branch":"feature/x"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"feature/x","access":"readable"}]}`,
	}
	if _, err := buildReviewerWorkspaceRequest(context.Background(), reviewerTask,
		&model.Run{RunID: "run-empty-row-rev", TaskID: reviewerTask.TaskID, WorkerRole: "ReviewerWorker"},
		ReviewerRunnerSettings{ResolveRepository: emptyRow}); err == nil || !strings.Contains(err.Error(), "cannot derive clone url") {
		t.Fatalf("reviewer dispatch should reject row without identity fields, got %v", err)
	}
}
