package events

import (
	"encoding/json"
	"strings"
	"testing"

	"sdlc-coordinator/internal/operationmarker"
)

const giteaIssueOpenedPayload = `{
  "action": "opened",
  "number": 7,
  "issue": {
    "id": 100, "number": 7, "title": "Fix widget", "body": "widget is broken",
    "state": "open", "html_url": "https://gitea.example/group/repo/issues/7",
    "updated_at": "2026-08-27T00:00:00Z",
    "labels": [{"name": "developer-agent"}],
    "assignees": [{"id": 7101, "login": "alice", "full_name": "Alice"}]
  },
  "repository": {
    "id": 42, "full_name": "group/repo", "clone_url": "https://gitea.example/group/repo.git",
    "html_url": "https://gitea.example/group/repo", "default_branch": "main"
  }
}`

const giteaPROpenedPayload = `{
  "action": "opened",
  "number": 12,
  "pull_request": {
    "id": 200, "number": 12, "title": "Fix widget", "body": "fixes #7",
    "state": "open", "merged": false,
    "html_url": "https://gitea.example/group/repo/pulls/12",
    "head": {"ref": "fix/widget", "sha": "abc123def"},
    "base": {"ref": "main", "sha": "def456abc"}
  },
  "repository": {
    "id": 42, "full_name": "group/repo", "clone_url": "https://gitea.example/group/repo.git",
    "default_branch": "main"
  }
}`

func TestGiteaNormalizerSource(t *testing.T) {
	if (GiteaNormalizer{}).Source() != "gitea" {
		t.Fatal("source must be gitea")
	}
}

func TestGiteaNormalizerMapsIssueOpenedToWorkItem(t *testing.T) {
	result, err := GiteaNormalizer{}.Normalize([]byte(giteaIssueOpenedPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event == nil {
		t.Fatalf("expected normalized event, got ignored=%v reason=%s", result.Ignored, result.IgnoreReason)
	}
	event := result.Event
	if event.Kind != EventKindWorkItemUpdated || event.Source != "gitea" {
		t.Fatalf("unexpected kind/source: %s/%s", event.Source, event.Kind)
	}
	if event.ProjectID != "42" || event.ProjectPath != "group/repo" {
		t.Fatalf("unexpected project identity: %#v", event)
	}
	if event.ObjectIID != 7 || event.Title != "Fix widget" {
		t.Fatalf("unexpected issue fields: %#v", event)
	}
	if event.RepositoryURL != "https://gitea.example/group/repo.git" {
		t.Fatalf("unexpected repo url: %s", event.RepositoryURL)
	}
	if event.DedupeKey != "gitea:issue-develop:42:7" {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
	if assigneeIDs := metadataIntSlice(event.Metadata, "assignee_ids"); len(assigneeIDs) != 1 || assigneeIDs[0] != 7101 {
		t.Fatalf("unexpected assignee ids: %#v", event.Metadata)
	}
}

func TestGiteaNormalizerIgnoresIssueWithoutDeveloperAgentLabel(t *testing.T) {
	payload := strings.Replace(giteaIssueOpenedPayload, `{"name": "developer-agent"}`, `{"name": "bug"}`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored {
		t.Fatalf("expected ignored for missing label, got %#v", result.Event)
	}
}

func TestGiteaNormalizerMapsPullRequestOpenedToChangeRequest(t *testing.T) {
	result, err := GiteaNormalizer{}.Normalize([]byte(giteaPROpenedPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.Kind != EventKindChangeRequestUpdated {
		t.Fatalf("expected change_request_updated, got %#v", result)
	}
	if event.HeadSHA != "abc123def" || event.SourceBranch != "fix/widget" || event.TargetBranch != "main" {
		t.Fatalf("unexpected review fields: %#v", event)
	}
	if !strings.HasPrefix(event.DedupeKey, "gitea:mr-review:42:12:") {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
	// base_sha 语义=增量起点(上一版源分支头);opened 无增量起点为空,
	// 目标分支 tip(def456abc)不得混入。
	if event.BaseSHA != "" {
		t.Fatalf("unexpected base sha: %s", event.BaseSHA)
	}
}

// synchronized 是源分支新 Push:动作词归一为 update,顶层 before(若有)是
// 增量起点;无 before 时优雅降级为空(任务工厂回落完整审查)。
func TestGiteaNormalizerMapsPullRequestSynchronizedToUpdate(t *testing.T) {
	withBefore := strings.Replace(giteaPROpenedPayload, `"action": "opened"`,
		`"action": "synchronized", "before": "old123abc"`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(withBefore))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.EventAction != "update" || event.BaseSHA != "old123abc" {
		t.Fatalf("expected update action with before as increment base, got %#v", event)
	}
	if metadataString(event.Metadata, "raw_action") != "synchronized" {
		t.Fatalf("expected raw action preserved, got %#v", event.Metadata)
	}

	withoutBefore := strings.Replace(giteaPROpenedPayload, `"action": "opened"`, `"action": "synchronized"`, 1)
	result, err = GiteaNormalizer{}.Normalize([]byte(withoutBefore))
	if err != nil {
		t.Fatalf("normalize without before: %v", err)
	}
	if event = result.Event; event == nil || event.EventAction != "update" || event.BaseSHA != "" {
		t.Fatalf("expected update action with empty increment base, got %#v", event)
	}
}

func TestGiteaNormalizerIgnoresPullRequestWithoutDeliveryBranch(t *testing.T) {
	payload := strings.Replace(giteaPROpenedPayload, `"ref": "fix/widget"`, `"ref": "main"`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored {
		t.Fatalf("expected ignored for non-delivery branch, got %#v", result.Event)
	}
}

func TestGiteaNormalizerMapsPullRequestClosedLifecycle(t *testing.T) {
	closedMerged := strings.Replace(giteaPROpenedPayload, `"action": "opened"`, `"action": "closed"`, 1)
	closedMerged = strings.Replace(closedMerged, `"merged": false`, `"merged": true`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(closedMerged))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.Kind != EventKindMergeRequestLifecycleChanged {
		t.Fatalf("expected lifecycle event, got %#v", result)
	}
	if metadataString(event.Metadata, "lifecycle_state") != "merged" {
		t.Fatalf("expected merged lifecycle state, got %#v", event.Metadata)
	}
	// 合并与关闭共用动作词 closed;归一为下游 switch 认的 merge/close。
	if event.EventAction != "merge" || event.HeadSHA != "abc123def" {
		t.Fatalf("unexpected lifecycle fields: %#v", event)
	}
	if metadataString(event.Metadata, "raw_action") != "closed" {
		t.Fatalf("expected raw action preserved, got %#v", event.Metadata)
	}
}

func giteaCommentPayload(isPR bool, body string) []byte {
	issuePart := `"labels": [{"name": "developer-agent"}], "state": "open",
    "html_url": "https://gitea.example/group/repo/issues/7",
    "pull_request": {"url": "https://gitea.example/group/repo/pulls/12"},`
	if !isPR {
		issuePart = `"labels": [{"name": "developer-agent"}], "state": "open",
    "html_url": "https://gitea.example/group/repo/issues/7",`
	}
	return []byte(`{
  "action": "created",
  "number": 7,
  "issue": {
    "id": 100, "number": 7, "title": "Fix widget", "body": "widget is broken", ` + issuePart + `
    "user": {"id": 71, "login": "author1", "full_name": "Author One"},
    "assignees": [{"id": 7101, "login": "alice"}]
  },
  "comment": {
    "id": 500, "body": ` + quoteJSON(body) + `,
    "html_url": "https://gitea.example/group/repo/issues/7#issuecomment-500",
    "user": {"id": 3, "login": "reviewer1", "full_name": "Reviewer One", "email": "reviewer@example.com"}
  },
  "repository": {
    "id": 42, "full_name": "group/repo", "clone_url": "https://gitea.example/group/repo.git",
    "default_branch": "main"
  }
}`)
}

func quoteJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestGiteaNormalizerMapsPRCommentToRework(t *testing.T) {
	result, err := GiteaNormalizer{}.Normalize(giteaCommentPayload(true, "请调整实现，状态枚举缺少 done。"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.Kind != EventKindMergeRequestReworkRequested {
		t.Fatalf("expected rework event, got %#v", result)
	}
	if event.ObjectIID != 12 {
		t.Fatalf("expected PR iid from issue pull_request link, got %d", event.ObjectIID)
	}
	if noteID, ok := metadataPositiveInt(event.Metadata["note_id"]); !ok || noteID != 500 {
		t.Fatalf("unexpected note id: %#v", event.Metadata)
	}
	if metadataString(event.Metadata, "resume_context") == "" {
		t.Fatalf("expected resume context, got %#v", event.Metadata)
	}
}

// 形状对齐 Gitea 真实 PR 评论载荷(references/Gitea-Webhook-请求体json/PR事件/PR评论创建.json):
// 顶层带完整 pull_request 对象,issue.pull_request 只是 PullRequestMeta。
var giteaPRCommentWithPullRequestPayload = []byte(`{
  "action": "created",
  "is_pull": true,
  "issue": {
    "id": 100, "number": 7, "title": "Fix widget", "body": "widget is broken",
    "state": "open",
    "html_url": "https://gitea.example/group/repo/pulls/12",
    "pull_request": {"merged": false, "html_url": "https://gitea.example/group/repo/pulls/12"}
  },
  "pull_request": {
    "id": 200, "number": 12, "title": "Fix widget", "body": "fixes #7",
    "state": "open", "merged": false,
    "html_url": "https://gitea.example/group/repo/pulls/12",
    "head": {"ref": "fix/widget", "sha": "abc123def"},
    "base": {"ref": "main", "sha": "def456abc"}
  },
  "comment": {
    "id": 500, "body": ` + quoteJSON("请调整实现，状态枚举缺少 done。") + `,
    "html_url": "https://gitea.example/group/repo/pulls/12#issuecomment-500",
    "user": {"id": 3, "login": "reviewer1", "full_name": "Reviewer One", "email": "reviewer@example.com"}
  },
  "repository": {
    "id": 42, "full_name": "group/repo", "clone_url": "https://gitea.example/group/repo.git",
    "default_branch": "main"
  }
}`)

// PR 评论载荷顶层自带完整 pull_request 对象(Gitea 1.26.4 实测),rework 事件应直读
// head/base 快照定位字段,补齐派发所需的 source branch 与 expected head sha。
func TestGiteaNormalizerMapsPRCommentWithPullRequestDetails(t *testing.T) {
	result, err := GiteaNormalizer{}.Normalize(giteaPRCommentWithPullRequestPayload)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.Kind != EventKindMergeRequestReworkRequested {
		t.Fatalf("expected rework event, got %#v", result)
	}
	if event.HeadSHA != "abc123def" || event.SourceBranch != "fix/widget" || event.TargetBranch != "main" {
		t.Fatalf("unexpected pull request details: %#v", event)
	}
	if event.DedupeKey != "gitea:mr-rework:42:12" {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
}

// 载荷缺失顶层 pull_request 时保持现状:字段留空、事件照常创建——分支由写回
// enrichment 兜底、head SHA 由 workspace 契约兜底,fail-closed 语义不变。
func TestGiteaNormalizerLeavesReworkFieldsEmptyWithoutPullRequestObject(t *testing.T) {
	result, err := GiteaNormalizer{}.Normalize(giteaCommentPayload(true, "请调整实现，状态枚举缺少 done。"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.Kind != EventKindMergeRequestReworkRequested {
		t.Fatalf("expected rework event, got %#v", result)
	}
	if event.HeadSHA != "" || event.SourceBranch != "" || event.TargetBranch != "" {
		t.Fatalf("fields must stay empty without top-level pull request, got %#v", event)
	}
}

func TestGiteaNormalizerMapsIssueCommentToResume(t *testing.T) {
	payload := strings.Replace(string(giteaCommentPayload(false, "补充:状态枚举为 todo/doing/done。")),
		`"user": {"id": 3, "login": "reviewer1", "full_name": "Reviewer One", "email": "reviewer@example.com"}`,
		`"user": {"id": 71, "login": "author1", "full_name": "Author One"}`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.Kind != EventKindDeveloperResumeRequested {
		t.Fatalf("expected resume event, got %#v", result)
	}
	if metadataString(event.Metadata, "resume_context") == "" {
		t.Fatalf("expected resume context, got %#v", event.Metadata)
	}
}

func TestGiteaNormalizerReceiptPrefersDeliveryID(t *testing.T) {
	receipt := GiteaNormalizer{}.Receipt([]byte(`{}`), WebhookMetadata{EventType: "issues", DeliveryID: "d-1"})
	if receipt.DedupeKey != "gitea:webhook:delivery:d-1" {
		t.Fatalf("unexpected delivery dedupe key: %s", receipt.DedupeKey)
	}

	receipt = GiteaNormalizer{}.Receipt([]byte(giteaIssueOpenedPayload), WebhookMetadata{EventType: "issues"})
	if receipt.DedupeKey == "" || strings.Contains(receipt.DedupeKey, "delivery") {
		t.Fatalf("expected payload-derived dedupe key, got %q", receipt.DedupeKey)
	}
	if receipt.ExternalID != "42:7" {
		t.Fatalf("unexpected external id: %s", receipt.ExternalID)
	}
}

// Gitea 真实载荷中 issue.pull_request 是 PullRequestMeta(merged/merged_at/html_url),
// 没有 url 键;PR 序号需回退到 payload.number,链接取 html_url。
func TestGiteaNormalizerMapsPRCommentToReworkFromGiteaPullRequestMeta(t *testing.T) {
	payload := strings.Replace(string(giteaCommentPayload(true, "请调整实现，状态枚举缺少 done。")),
		`"pull_request": {"url": "https://gitea.example/group/repo/pulls/12"}`,
		`"pull_request": {"merged": false, "html_url": "https://gitea.example/group/repo/pulls/12"}`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.Kind != EventKindMergeRequestReworkRequested {
		t.Fatalf("expected rework event, got %#v", result)
	}
	if event.ObjectIID != 12 {
		t.Fatalf("expected PR iid 12, got %d", event.ObjectIID)
	}
	if event.ObjectURL != "https://gitea.example/group/repo/pulls/12" {
		t.Fatalf("expected pull request meta html_url, got %q", event.ObjectURL)
	}
}

// pull_request 链接完全缺失时,issue.html_url(PR 场景即 PR 页面)兜底,
// 否则 rework 请求会因 merge request url 为空被校验拒绝。
func TestGiteaNormalizerFallsBackToIssueURLForReworkLink(t *testing.T) {
	payload := strings.Replace(string(giteaCommentPayload(true, "请调整实现。")),
		`"pull_request": {"url": "https://gitea.example/group/repo/pulls/12"}`,
		`"pull_request": {"merged": false}, "html_url": "https://gitea.example/group/repo/pulls/7"`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.Kind != EventKindMergeRequestReworkRequested {
		t.Fatalf("expected rework event, got %#v", result)
	}
	if event.ObjectURL != "https://gitea.example/group/repo/pulls/7" {
		t.Fatalf("expected issue html_url fallback, got %q", event.ObjectURL)
	}
}

func TestGiteaNormalizerIgnoresPRCommentOnClosedMergeRequest(t *testing.T) {
	payload := strings.Replace(string(giteaCommentPayload(true, "请调整实现。")),
		`"state": "open"`, `"state": "closed"`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.Event != nil {
		t.Fatalf("expected closed merge request comment to be ignored, got %#v", result)
	}
}

// 评论缺作者时不能 panic,也无法通过下游 requester 授权,应显式忽略。
func TestGiteaNormalizerIgnoresPRCommentWithoutAuthor(t *testing.T) {
	payload := strings.Replace(string(giteaCommentPayload(true, "请调整实现。")),
		`"user": {"id": 3, "login": "reviewer1", "full_name": "Reviewer One", "email": "reviewer@example.com"}`,
		`"user": null`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize panicked or failed: %v", err)
	}
	if !result.Ignored || result.Event != nil {
		t.Fatalf("expected comment without author to be ignored, got %#v", result)
	}
}

// GitLab 侧 rework 预授权覆盖 MR 作者/assignee;Gitea 需从载荷对齐:
// PR 作者(issue.user)与 PR assignee(issue.assignees)评论视为已授权。
func TestGiteaNormalizerPreAuthorizesReworkRequesterFromMergeRequestRoles(t *testing.T) {
	cases := []struct {
		name          string
		commentUser   string
		preAuthorized bool
	}{
		{name: "merge request author", commentUser: `"id": 71, "login": "author1"`, preAuthorized: true},
		{name: "merge request assignee", commentUser: `"id": 7101, "login": "alice"`, preAuthorized: true},
		{name: "outside requester", commentUser: `"id": 3, "login": "reviewer1"`, preAuthorized: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := strings.Replace(string(giteaCommentPayload(true, "请调整实现。")),
				`"user": {"id": 3, "login": "reviewer1", "full_name": "Reviewer One", "email": "reviewer@example.com"}`,
				`"user": {`+testCase.commentUser+`}`, 1)
			result, err := GiteaNormalizer{}.Normalize([]byte(payload))
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			event := result.Event
			if event == nil || event.Kind != EventKindMergeRequestReworkRequested {
				t.Fatalf("expected rework event, got %#v", result)
			}
			preAuthorized, _ := event.Metadata["requester_pre_authorized"].(bool)
			if preAuthorized != testCase.preAuthorized {
				t.Fatalf("expected requester_pre_authorized=%v, got %#v", testCase.preAuthorized, event.Metadata)
			}
		})
	}
}

// lifecycle 终结按 dedupe key 定位 rework 任务,键必须与 rework 事件一致
// (GitLab 侧即复用 mr-rework 键);否则 MR 合并/关闭后 rework 任务永远无法收尾。
func TestGiteaNormalizerAlignsLifecycleDedupeKeyWithRework(t *testing.T) {
	closed := strings.Replace(giteaPROpenedPayload, `"action": "opened"`, `"action": "closed"`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(closed))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.Kind != EventKindMergeRequestLifecycleChanged {
		t.Fatalf("expected lifecycle event, got %#v", result)
	}
	if event.DedupeKey != "gitea:mr-rework:42:12" {
		t.Fatalf("expected lifecycle dedupe key aligned with rework key gitea:mr-rework:42:12, got %q", event.DedupeKey)
	}
}

// UI 场景先建 issue 后打标签:label_updated 才是事实上的启动信号,
// 与带标签的 opened 同等对待(任务键相同,CreateTask 幂等)。
func TestGiteaNormalizerMapsIssueLabelUpdatedToWorkItem(t *testing.T) {
	payload := strings.Replace(giteaIssueOpenedPayload, `"action": "opened"`, `"action": "label_updated"`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event == nil || result.Event.Kind != EventKindWorkItemUpdated {
		t.Fatalf("expected work item event from label_updated, got %#v", result)
	}
}

func TestGiteaNormalizerIgnoresIssueLabelUpdatedWithoutDeveloperAgentLabel(t *testing.T) {
	payload := strings.Replace(giteaIssueOpenedPayload, `"action": "opened"`, `"action": "label_updated"`, 1)
	payload = strings.Replace(payload, `{"name": "developer-agent"}`, `{"name": "bug"}`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.Event != nil {
		t.Fatalf("expected label_updated without developer-agent to be ignored, got %#v", result)
	}
}

// 回归：写回评论的标记前缀已换代为 agent-coordinator-operation，
// Gitea 侧过滤器必须同样识别（否则自家审查评论会回流成 rework 任务，形成循环）。
func TestGiteaPRCommentWithCurrentOperationMarkerIsIgnored(t *testing.T) {
	marker := operationmarker.New("codehost:task-1:run-1:0:create_merge_request_note")
	body := "审查已完成，结论：建议合并。\n\n" + marker

	result, err := GiteaNormalizer{}.Normalize(giteaCommentPayload(true, body))
	if err != nil {
		t.Fatalf("normalize operation comment: %v", err)
	}
	if result.Event != nil || !strings.Contains(result.IgnoreReason, "coordinator operation comment") {
		t.Fatalf("current-generation operation marker must be ignored, got event=%v reason=%q", result.Event, result.IgnoreReason)
	}
}

// resume 请求者白名单:作者 ∪ 指派人 ∪ 平台白名单;陌生人触发应被忽略。
func TestGiteaNormalizerIgnoresResumeFromUnauthorizedRequester(t *testing.T) {
	result, err := GiteaNormalizer{}.Normalize(giteaCommentPayload(false, "补充:状态枚举为 todo/doing/done。"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.IgnoreReason != "resume requester is not authorized" {
		t.Fatalf("expected unauthorized requester to be ignored, got %#v", result)
	}
}

func TestGiteaNormalizerAllowsResumeForAssignee(t *testing.T) {
	payload := strings.Replace(string(giteaCommentPayload(false, "补充:状态枚举为 todo/doing/done。")),
		`"user": {"id": 3, "login": "reviewer1", "full_name": "Reviewer One", "email": "reviewer@example.com"}`,
		`"user": {"id": 7101, "login": "alice", "full_name": "Assignee One"}`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event == nil {
		t.Fatalf("expected assignee resume to be accepted, got %#v", result)
	}
}

func TestGiteaNormalizerAllowsResumeForPlatformAllowlist(t *testing.T) {
	normalizer := NewGiteaNormalizer(WithGiteaReworkAllowedUserIDs([]int{3}))
	result, err := normalizer.Normalize(giteaCommentPayload(false, "补充:状态枚举为 todo/doing/done。"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event == nil {
		t.Fatalf("expected allowlisted requester resume to be accepted, got %#v", result)
	}
}

// 作者身份不可得且无指派人时 fail-closed 拒绝:「仅作者」边界无法确立的触发
// 不放行(真实载荷作者恒可得,该分支为防御性收口)。
func TestGiteaNormalizerRejectsResumeWhenAuthorUnavailable(t *testing.T) {
	payload := string(giteaCommentPayload(false, "补充:状态枚举为 todo/doing/done。"))
	payload = strings.Replace(payload, `"user": {"id": 71, "login": "author1", "full_name": "Author One"},`, `"user": null,`, 1)
	payload = strings.Replace(payload, `"assignees": [{"id": 7101, "login": "alice"}]`, `"assignees": []`, 1)
	result, err := GiteaNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.IgnoreReason != "resume requester is not authorized" {
		t.Fatalf("expected unauthorized ignore, got %#v", result)
	}
}
