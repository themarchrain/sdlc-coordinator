package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/adapters/gitea"
	"sdlc-coordinator/internal/adapters/gitlab"
	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/people"
	"sdlc-coordinator/internal/registeredrepo"

	"go.uber.org/zap"
)

func hmacSHA256Hex(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// giteaIssueOpenedPayload 构造 Gitea issue opened 载荷(带 developer-agent 标签,
// 是任务创建信号);repository.id 是平台数字仓库 ID。
func giteaIssueOpenedPayload(repositoryID int64, fullPath string) []byte {
	rawID, _ := json.Marshal(repositoryID)
	return []byte(`{
		"action": "opened",
		"issue": {
			"number": 7,
			"state": "open",
			"title": "Gitea 侧需求",
			"body": "developer agent 需求",
			"html_url": "https://gitea.example/` + fullPath + `/issues/7",
			"updated_at": "2026-09-01T00:00:00Z",
			"labels": [{"name": "developer-agent"}],
			"assignees": [{"id": 7, "login": "alice"}]
		},
		"repository": {
			"id": ` + string(rawID) + `,
			"full_name": "` + fullPath + `",
			"clone_url": "https://gitea.example/` + fullPath + `.git",
			"default_branch": "main"
		}
	}`)
}

// TestMultiPlatformCoexistence 是多平台并存的 headline:GitLab 仓与 Gitea 仓同
// path、同数字 project id 并存且启用——
//  1. 各自 webhook 各自验签(跨平台 secret 401),任务各落各的仓;
//  2. 各自任务的写回 client 各走各的平台(HTTP 落点 + client 类型断言);
//  3. 同数字 assignee id 在两平台解析为不同人员(三键身份隔离);
//  4. 全局 codehost 配置全程为零(降级为默认值的运行时零消费证明)。
func TestMultiPlatformCoexistence(t *testing.T) {
	ctx := context.Background()
	var gitlabHits, giteaHits atomic.Int64
	gitlabBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		gitlabHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"iid":11,"id":11,"web_url":"https://gitlab.examplegroup/project/issues/11"}`))
	}))
	defer gitlabBackend.Close()
	giteaBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		giteaHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"iid":22,"id":22,"html_url":"https://gitea.example/group/project/issues/22"}`))
	}))
	defer giteaBackend.Close()

	db := openAppTestDB(t)
	application, err := New(config.Config{HTTPAddr: ":0"}, zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app without global codehost config: %v", err)
	}

	repoStore := registeredrepo.NewStore(db)
	// 同 path、同数字 project id:身份唯一性只在 provider 作用域内成立。
	seeded := []*registeredrepo.RegisteredRepository{
		{
			RepositoryID: "repo_gl", DisplayName: "gitlab 仓", Provider: "gitlab", BaseURL: gitlabBackend.URL,
			CodeHostProjectID: "15594", PathWithNamespace: "demo-group/lite-list",
			Token: "token-gl", WebhookSecret: "secret-gl", TokenScopeAttested: true,
			Status: registeredrepo.StatusEnabled,
		},
		{
			RepositoryID: "repo_gt", DisplayName: "gitea 仓", Provider: "gitea", BaseURL: giteaBackend.URL,
			CodeHostProjectID: "15594", PathWithNamespace: "demo-group/lite-list",
			Token: "token-gt", WebhookSecret: "secret-gt", TokenScopeAttested: true,
			Status: registeredrepo.StatusEnabled,
		},
	}
	for _, repo := range seeded {
		if err := repoStore.Create(ctx, repo); err != nil {
			t.Fatalf("seed %s: %v", repo.RepositoryID, err)
		}
	}

	// 1) webhook 验签按 provider 作用域隔离:同 path 仓各匹配各的 secret。
	gitlabPayload := readAppFixture(t, "../events/testdata/gitlab/merge_request_opened.json")
	gitlabAccepted := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitlab", gitlabPayload,
		map[string]string{"X-Gitlab-Token": "secret-gl"})
	if gitlabAccepted.Code != http.StatusAccepted {
		t.Fatalf("gitlab webhook with own secret must be accepted, got %d: %s", gitlabAccepted.Code, gitlabAccepted.Body.String())
	}
	gitlabCross := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitlab", gitlabPayload,
		map[string]string{"X-Gitlab-Token": "secret-gt"})
	if gitlabCross.Code != http.StatusUnauthorized {
		t.Fatalf("gitlab endpoint with gitea-scoped secret must be 401, got %d", gitlabCross.Code)
	}

	giteaPayload := giteaIssueOpenedPayload(15594, "demo-group/lite-list")
	giteaAccepted := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitea", giteaPayload,
		map[string]string{"X-Gitea-Signature": hmacSHA256Hex("secret-gt", giteaPayload)})
	if giteaAccepted.Code != http.StatusAccepted {
		t.Fatalf("gitea webhook with own secret must be accepted, got %d: %s", giteaAccepted.Code, giteaAccepted.Body.String())
	}
	giteaCross := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitea", giteaPayload,
		map[string]string{"X-Gitea-Signature": hmacSHA256Hex("secret-gl", giteaPayload)})
	if giteaCross.Code != http.StatusUnauthorized {
		t.Fatalf("gitea endpoint with gitlab-scoped secret must be 401, got %d", giteaCross.Code)
	}

	// 各自任务绑定各自的仓(同 project id 跨平台不串仓)。
	type taskRow struct {
		TaskID       string
		RepositoryID *string
	}
	var rows []taskRow
	if err := db.Table("tasks").Select("task_id, repository_id").Find(&rows).Error; err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	bound := map[string]string{}
	for _, row := range rows {
		if row.RepositoryID == nil {
			t.Fatalf("task %s created without repository binding", row.TaskID)
		}
		bound[row.TaskID] = *row.RepositoryID
	}
	seen := map[string]int{"repo_gl": 0, "repo_gt": 0}
	for _, repoID := range bound {
		if _, ok := seen[repoID]; !ok {
			t.Fatalf("unexpected task bound to %s", repoID)
		}
		seen[repoID]++
	}
	for repoID, count := range seen {
		if count != 1 {
			t.Fatalf("expected exactly one task bound to %s, got %d (tasks=%v)", repoID, count, bound)
		}
	}

	// 2) 写回按仓行分派:同 project id 的两任务,client 各打各的平台后端。
	resolver := registeredrepo.NewCodeHostResolver(repoStore, nil)
	writeback := &model.Writeback{ProjectID: "15594"}
	glID, gtID := "repo_gl", "repo_gt"
	gitlabBackendClient, err := resolver.Resolve(ctx, &model.Task{TaskID: "t-gl", RepositoryID: &glID}, writeback)
	if err != nil {
		t.Fatalf("resolve gitlab writeback: %v", err)
	}
	if _, ok := gitlabBackendClient.(*gitlab.Client); !ok {
		t.Fatalf("expected gitlab client for gitlab repo, got %T", gitlabBackendClient)
	}
	if _, err := gitlabBackendClient.CreateIssue(ctx, "15594", codehost.CreateIssueRequest{Title: "isolation probe"}); err != nil && gitlabHits.Load() == 0 {
		t.Fatalf("gitlab writeback did not reach gitlab backend: %v", err)
	}
	giteaBackendClient, err := resolver.Resolve(ctx, &model.Task{TaskID: "t-gt", RepositoryID: &gtID}, writeback)
	if err != nil {
		t.Fatalf("resolve gitea writeback: %v", err)
	}
	if _, ok := giteaBackendClient.(*gitea.Client); !ok {
		t.Fatalf("expected gitea client for gitea repo, got %T", giteaBackendClient)
	}
	if _, err := giteaBackendClient.CreateIssue(ctx, "demo-group/lite-list", codehost.CreateIssueRequest{Title: "isolation probe"}); err != nil && giteaHits.Load() == 0 {
		t.Fatalf("gitea writeback did not reach gitea backend: %v", err)
	}
	if gitlabHits.Load() != 1 || giteaHits.Load() != 1 {
		t.Fatalf("each platform backend must be hit exactly once, gitlab=%d gitea=%d", gitlabHits.Load(), giteaHits.Load())
	}

	// 3) mention 三键隔离:同数字 id 在两平台解析为不同人员。
	if err := people.NewRepository(db).Migrate(); err != nil {
		t.Fatalf("migrate people: %v", err)
	}
	peopleRepo := people.NewRepository(db)
	if err := peopleRepo.Create(ctx, &people.Person{
		PersonID: "person-gl", DisplayName: "艾丽斯-GitLab", Email: "alice-gl@example.com", Enabled: true,
	}, []people.CodeHostIdentity{{IdentityID: "id-gl", PersonID: "person-gl", Provider: "gitlab",
		CodeHostBaseURL: gitlabBackend.URL, ExternalUserID: "7", Username: "alice"}}); err != nil {
		t.Fatalf("seed gitlab person: %v", err)
	}
	if err := peopleRepo.Create(ctx, &people.Person{
		PersonID: "person-gt", DisplayName: "艾丽斯-Gitea", Email: "alice-gt@example.com", Enabled: true,
	}, []people.CodeHostIdentity{{IdentityID: "id-gt", PersonID: "person-gt", Provider: "gitea",
		CodeHostBaseURL: giteaBackend.URL, ExternalUserID: "7", Username: "alice"}}); err != nil {
		t.Fatalf("seed gitea person: %v", err)
	}
	directory := newPeopleMentionDirectory(people.NewService(peopleRepo), repoStore)
	gitlabMentions := directory.ResolveMentionNames(ctx, "repo_gl", []int{7})
	if got := gitlabMentions[7].DisplayName; got != "艾丽斯-GitLab" {
		t.Fatalf("gitlab-scoped mention=%q", got)
	}
	giteaMentions := directory.ResolveMentionNames(ctx, "repo_gt", []int{7})
	if got := giteaMentions[7].DisplayName; got != "艾丽斯-Gitea" {
		t.Fatalf("gitea-scoped mention=%q", got)
	}
}

// TestLegacyRepositoryRowsAreBackfilledOnStartup 启动时为缺少平台身份的仓行补齐
// provider/base_url,且不改变 enabled 状态。
func TestLegacyRepositoryRowsAreBackfilledOnStartup(t *testing.T) {
	db := openAppTestDB(t)
	repoStore := registeredrepo.NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	legacy := &registeredrepo.RegisteredRepository{
		RepositoryID: "repo_legacy", DisplayName: "legacy", Provider: "", BaseURL: "",
		CodeHostProjectID: "15594", PathWithNamespace: "demo-group/lite-list",
		Token: "token-legacy", WebhookSecret: "secret-legacy", TokenScopeAttested: true,
		Status: registeredrepo.StatusEnabled,
	}
	if err := repoStore.Create(context.Background(), legacy); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	application, err := New(config.Config{
		HTTPAddr:         ":0",
		CodeHostProvider: "gitlab",
		CodeHostBaseURL:  "https://gitlab.example.com",
	}, zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	_ = application

	backfilled, err := repoStore.Get(context.Background(), "repo_legacy")
	if err != nil {
		t.Fatalf("reload legacy row: %v", err)
	}
	if backfilled.Provider != "gitlab" || backfilled.BaseURL != "https://gitlab.example.com" {
		t.Fatalf("legacy row not backfilled: provider=%q base_url=%q", backfilled.Provider, backfilled.BaseURL)
	}
	if backfilled.Status != registeredrepo.StatusEnabled {
		t.Fatalf("backfill must not change status, got %s", backfilled.Status)
	}

	// 重启幂等:第二次启动回填零行、索引 IF NOT EXISTS,仓行与状态不变。
	if _, err := New(config.Config{
		HTTPAddr:         ":0",
		CodeHostProvider: "gitlab",
		CodeHostBaseURL:  "https://gitlab.example.com",
	}, zap.NewNop(), db); err != nil {
		t.Fatalf("second startup must be idempotent: %v", err)
	}
	rerun, err := repoStore.Get(context.Background(), "repo_legacy")
	if err != nil {
		t.Fatalf("reload after restart: %v", err)
	}
	if rerun.Provider != "gitlab" || rerun.BaseURL != "https://gitlab.example.com" || rerun.Status != registeredrepo.StatusEnabled {
		t.Fatalf("restart must not alter repository identity: %+v", rerun)
	}
}
