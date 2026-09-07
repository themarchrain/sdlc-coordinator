package events

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	storerepo "sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

type memoryRepoGate struct {
	repos []registeredrepo.RegisteredRepository
}

func (gate memoryRepoGate) GetByProjectID(_ context.Context, provider, projectID string) (*registeredrepo.RegisteredRepository, error) {
	for i := range gate.repos {
		if gate.repos[i].CodeHostProjectID == projectID && strings.EqualFold(gate.repos[i].Provider, provider) {
			return &gate.repos[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func processWithRepos(t *testing.T, payload []byte, repos []registeredrepo.RegisteredRepository) *ProcessResult {
	t.Helper()
	processor, _, _ := newProcessorFixture(t)
	processor.repos = memoryRepoGate{repos: repos}
	result, err := processor.ProcessGitLabWebhook(context.Background(), payload)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	return result
}

// TestProcessorDefersUnresolvedIdentityAsNotEnabled 锁定 deferred 裁决:webhook
// secret 按仓验签后,真未注册的仓在验签处已被 401 拒绝、不会落库;processor 内
// not-found 只剩「注册仓存在但平台身份未探测完成(configuring)」的残余——按
// project_id 查不到时并入 repository_not_enabled(可重放状态),启用补齐身份后
// 同 UUID 重放恢复评估。
func TestProcessorDefersUnresolvedIdentityAsNotEnabled(t *testing.T) {
	result := processWithRepos(t, []byte(gitLabMROpenPayload), nil)
	if result.Ignored || result.IgnoreReason != "repository_not_enabled" {
		t.Fatalf("%+v", result)
	}
	if result.Event == nil || result.Event.Status != model.InboundEventStatusDeferred {
		t.Fatalf("expected deferred event, got %#v", result.Event)
	}
	if result.Event.RepositoryID != nil {
		t.Fatal("unresolved identity must not set repository_id")
	}
	if result.Task != nil {
		t.Fatal("must not create task")
	}
}

func TestProcessorCreatesTaskForEnabledRepositoryOnly(t *testing.T) {
	repos := []registeredrepo.RegisteredRepository{
		{RepositoryID: "repo_a", Provider: "gitlab", CodeHostProjectID: "24207", PathWithNamespace: "group/project", Status: registeredrepo.StatusEnabled},
		{RepositoryID: "repo_b", Provider: "gitlab", CodeHostProjectID: "2", PathWithNamespace: "group/b", Status: registeredrepo.StatusDisabled},
	}
	a := processWithRepos(t, []byte(gitLabMROpenPayload), repos)
	if a.Task == nil || a.Task.RepositoryID == nil || *a.Task.RepositoryID != "repo_a" {
		t.Fatalf("task repo %+v", a.Task)
	}
	if a.Event.RepositoryID == nil || *a.Event.RepositoryID != "repo_a" {
		t.Fatal("inbound event missing repository_id")
	}

	disabledPayload := strings.ReplaceAll(gitLabMROpenPayload, `"id": 24207`, `"id": 2`)
	disabledPayload = strings.ReplaceAll(disabledPayload, "group/project", "group/b")
	b := processWithRepos(t, []byte(disabledPayload), repos)
	if b.Ignored || b.Task != nil {
		t.Fatal("disabled repo must not create task")
	}
	if b.Event == nil || b.Event.Status != model.InboundEventStatusDeferred {
		t.Fatalf("expected deferred event for disabled repo, got %#v", b.Event)
	}
	if b.Event.RepositoryID == nil || *b.Event.RepositoryID != "repo_b" {
		t.Fatal("disabled event must keep repository_id")
	}
}

func TestProcessorIgnoresIdentityMismatch(t *testing.T) {
	repos := []registeredrepo.RegisteredRepository{
		{RepositoryID: "repo_a", Provider: "gitlab", CodeHostProjectID: "24207", PathWithNamespace: "group/other", Status: registeredrepo.StatusEnabled},
	}
	result := processWithRepos(t, []byte(gitLabMROpenPayload), repos)
	if !result.Ignored || result.IgnoreReason != "repository_identity_mismatch" || result.Task != nil {
		t.Fatalf("%+v", result)
	}
	if result.Event.RepositoryID == nil || *result.Event.RepositoryID != "repo_a" {
		t.Fatal("mismatch must keep known repository_id")
	}
}

func TestProcessorNilGateKeepsLegacyAdmission(t *testing.T) {
	processor, _, _ := newProcessorFixture(t)
	result, err := processor.ProcessGitLabWebhook(context.Background(), []byte(gitLabMROpenPayload))
	if err != nil {
		t.Fatal(err)
	}
	if result.Ignored || result.Task == nil {
		t.Fatalf("legacy processor should still create tasks: %+v", result)
	}
}

func TestMemoryRepoGateNotFound(t *testing.T) {
	_, err := memoryRepoGate{}.GetByProjectID(context.Background(), "gitlab", "missing")
	if !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, storerepo.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

// TestProcessorScopesProjectIDLookupBySource 锁定 provider 作用域:同数字
// project id 跨平台实例可能撞号,事件必须按 Source 归属对应平台的仓。
func TestProcessorScopesProjectIDLookupBySource(t *testing.T) {
	repos := []registeredrepo.RegisteredRepository{
		{RepositoryID: "repo_gl", Provider: "gitlab", CodeHostProjectID: "24207", PathWithNamespace: "group/project", Status: registeredrepo.StatusEnabled},
		{RepositoryID: "repo_gt", Provider: "gitea", CodeHostProjectID: "24207", PathWithNamespace: "group/project", Status: registeredrepo.StatusEnabled},
	}
	processor, _, _ := newProcessorFixture(t)
	processor.repos = memoryRepoGate{repos: repos}

	gitlab, err := processor.ProcessGitLabWebhook(context.Background(), []byte(gitLabMROpenPayload))
	if err != nil {
		t.Fatal(err)
	}
	if gitlab.Task == nil || gitlab.Task.RepositoryID == nil || *gitlab.Task.RepositoryID != "repo_gl" {
		t.Fatalf("gitlab event must bind gitlab repo: %+v", gitlab.Task)
	}
}
