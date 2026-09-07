package events

import (
	"strconv"
	"strings"
	"testing"
)

const githubIssueOpenedPayload = `{
  "action": "opened",
  "issue": {
    "id": 100, "number": 7, "title": "Fix widget", "body": "widget is broken",
    "state": "open", "html_url": "https://github.com/org/repo/issues/7",
    "updated_at": "2026-08-28T00:00:00Z",
    "user": {"id": 71, "login": "author1"},
    "labels": [{"name": "developer-agent"}],
    "assignees": [{"id": 7101, "login": "alice"}]
  },
  "repository": {
    "id": 42, "full_name": "org/repo",
    "clone_url": "https://github.com/org/repo.git",
    "html_url": "https://github.com/org/repo",
    "default_branch": "main"
  }
}`

const githubPROpenedPayload = `{
  "action": "opened",
  "number": 12,
  "pull_request": {
    "id": 200, "number": 12, "title": "Fix widget", "body": "fixes #7",
    "state": "open", "merged": false,
    "html_url": "https://github.com/org/repo/pull/12",
    "head": {"ref": "fix/widget", "sha": "abc123def", "repo": {"full_name": "org/repo", "clone_url": "https://github.com/org/repo.git"}},
    "base": {"ref": "main", "sha": "def456abc", "repo": {"full_name": "org/repo"}},
    "user": {"id": 71, "login": "author1"},
    "assignees": [{"id": 7101, "login": "alice"}]
  },
  "repository": {
    "id": 42, "full_name": "org/repo",
    "clone_url": "https://github.com/org/repo.git",
    "default_branch": "main"
  }
}`

func TestGitHubNormalizerSource(t *testing.T) {
	if (GitHubNormalizer{}).Source() != "github" {
		t.Fatal("source must be github")
	}
}

func TestGitHubNormalizerMapsIssueOpenedToWorkItem(t *testing.T) {
	result, err := GitHubNormalizer{}.Normalize([]byte(githubIssueOpenedPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event == nil {
		t.Fatalf("expected normalized event, got ignored=%v reason=%s", result.Ignored, result.IgnoreReason)
	}
	event := result.Event
	if event.Kind != EventKindWorkItemUpdated || event.Source != "github" {
		t.Fatalf("unexpected kind/source: %s/%s", event.Source, event.Kind)
	}
	if event.ProjectID != "42" || event.ProjectPath != "org/repo" || event.ObjectIID != 7 {
		t.Fatalf("unexpected identity: %#v", event)
	}
	if event.RepositoryURL != "https://github.com/org/repo.git" || event.TargetBranch != "main" {
		t.Fatalf("unexpected repo facts: %#v", event)
	}
	if event.DedupeKey != "github:issue-develop:42:7" {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
	if ids := metadataIntSlice(event.Metadata, "assignee_ids"); len(ids) != 1 || ids[0] != 7101 {
		t.Fatalf("unexpected assignee ids: %#v", event.Metadata)
	}
}

// GitHub 与 Gitea UI 一样"先建后打标签":labeled 是事实上的启动信号。
func TestGitHubNormalizerMapsIssueLabeledToWorkItem(t *testing.T) {
	payload := strings.Replace(githubIssueOpenedPayload, `"action": "opened"`,
		`"action": "labeled", "label": {"name": "developer-agent"}`, 1)
	result, err := GitHubNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event == nil || result.Event.Kind != EventKindWorkItemUpdated {
		t.Fatalf("expected work item event from labeled, got %#v", result)
	}
}

func TestGitHubNormalizerIgnoresIssueWithoutDeveloperAgentLabel(t *testing.T) {
	payload := strings.Replace(githubIssueOpenedPayload, `{"name": "developer-agent"}`, `{"name": "bug"}`, 1)
	result, err := GitHubNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.Event != nil {
		t.Fatalf("expected ignored, got %#v", result)
	}
}

// labeled 的是其他标签时,即使 issue 已带 developer-agent 也不重复启动。
func TestGitHubNormalizerIgnoresLabeledWithOtherLabel(t *testing.T) {
	payload := strings.Replace(githubIssueOpenedPayload, `"action": "opened"`,
		`"action": "labeled", "label": {"name": "bug"}`, 1)
	result, err := GitHubNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.Event != nil {
		t.Fatalf("expected ignored, got %#v", result)
	}
}

func TestGitHubNormalizerMapsPullRequestOpenedToChangeRequest(t *testing.T) {
	result, err := GitHubNormalizer{}.Normalize([]byte(githubPROpenedPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	event := result.Event
	if event == nil || event.Kind != EventKindChangeRequestUpdated {
		t.Fatalf("expected change request event, got %#v", result)
	}
	if event.ObjectIID != 12 || event.ProjectID != "42" || event.ProjectPath != "org/repo" {
		t.Fatalf("unexpected identity: %#v", event)
	}
	if event.SourceBranch != "fix/widget" || event.TargetBranch != "main" || event.HeadSHA != "abc123def" {
		t.Fatalf("unexpected branch facts: %#v", event)
	}
	if event.DedupeKey != "github:mr-review:42:12:abc123def" {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
	if event.Metadata["author_username"] != "author1" {
		t.Fatalf("unexpected author metadata: %#v", event.Metadata)
	}
}

func TestGitHubNormalizerIgnoresPullRequestWithoutDeliveryBranch(t *testing.T) {
	headEqualsBase := strings.Replace(githubPROpenedPayload, `"head": {"ref": "fix/widget"`, `"head": {"ref": "main"`, 1)
	result, err := GitHubNormalizer{}.Normalize([]byte(headEqualsBase))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.Event != nil {
		t.Fatalf("expected head==base to be ignored, got %#v", result)
	}

	headOnDefault := strings.Replace(githubPROpenedPayload, `"ref": "fix/widget", "sha": "abc123def"`, `"ref": "main", "sha": "abc123def"`, 1)
	result, err = GitHubNormalizer{}.Normalize([]byte(headOnDefault))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.Event != nil {
		t.Fatalf("expected head on default branch to be ignored, got %#v", result)
	}
}

func TestGitHubNormalizerIgnoresCrossRepositoryPullRequest(t *testing.T) {
	payload := strings.Replace(githubPROpenedPayload, `"base": {"ref": "main", "sha": "def456abc", "repo": {"full_name": "org/repo"}}`,
		`"base": {"ref": "main", "sha": "def456abc", "repo": {"full_name": "other/repo"}}`, 1)
	result, err := GitHubNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.Event != nil {
		t.Fatalf("expected fork pull request to be ignored, got %#v", result)
	}
}

func TestGitHubNormalizerMapsPullRequestClosedLifecycle(t *testing.T) {
	closed := strings.Replace(githubPROpenedPayload, `"action": "opened"`, `"action": "closed"`, 1)
	closed = strings.Replace(closed, `"merged": false`, `"merged": true`, 1)
	result, err := GitHubNormalizer{}.Normalize([]byte(closed))
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
	if event.DedupeKey != "github:mr-rework:42:12" {
		t.Fatalf("expected lifecycle dedupe key aligned with rework key, got %q", event.DedupeKey)
	}
}

// GitHub 的 issue 与 PR 同号:PR 评论场景 issue.number 即 PR 号。
func githubCommentPayload(isPR bool, body string) []byte {
	issuePart := `"labels": [{"name": "developer-agent"}], "state": "open",
    "html_url": "https://github.com/org/repo/pull/12",
    "pull_request": {"html_url": "https://github.com/org/repo/pull/12", "url": "https://api.github.com/repos/org/repo/pulls/12", "merged": false},`
	issueNumber := 12
	if !isPR {
		issuePart = `"labels": [{"name": "developer-agent"}], "state": "open",
    "html_url": "https://github.com/org/repo/issues/7",`
		issueNumber = 7
	}
	return []byte(`{
  "action": "created",
  "issue": {
    "id": 100, "number": ` + strconv.Itoa(issueNumber) + `, "title": "Fix widget", "body": "widget is broken",
    "user": {"id": 71, "login": "author1"},
    "assignees": [{"id": 7101, "login": "alice"}], ` + issuePart + `
    "updated_at": "2026-08-28T00:00:00Z"
  },
  "comment": {
    "id": 500, "body": ` + quoteJSON(body) + `,
    "html_url": "https://github.com/org/repo/issues/7#issuecomment-500",
    "user": {"id": 3, "login": "reviewer1"}
  },
  "repository": {
    "id": 42, "full_name": "org/repo",
    "clone_url": "https://github.com/org/repo.git",
    "default_branch": "main"
  }
}`)
}

func TestGitHubNormalizerMapsPRCommentToRework(t *testing.T) {
	result, err := GitHubNormalizer{}.Normalize(githubCommentPayload(true, "请调整实现，状态枚举缺少 done。"))
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
	if event.ObjectURL != "https://github.com/org/repo/pull/12" {
		t.Fatalf("expected pull request url, got %q", event.ObjectURL)
	}
	if noteID, ok := metadataPositiveInt(event.Metadata["note_id"]); !ok || noteID != 500 {
		t.Fatalf("unexpected note id: %#v", event.Metadata)
	}
	if requesterID, ok := metadataPositiveInt(event.Metadata["requester_id"]); !ok || requesterID != 3 {
		t.Fatalf("unexpected requester id: %#v", event.Metadata)
	}
	if preAuthorized, _ := event.Metadata["requester_pre_authorized"].(bool); preAuthorized {
		t.Fatalf("expected outside requester not pre-authorized: %#v", event.Metadata)
	}
}

// PR 作者与 assignee 预授权,与 GitLab/Gitea 语义对齐。
func TestGitHubNormalizerPreAuthorizesReworkRequesterFromRoles(t *testing.T) {
	cases := []struct {
		name          string
		commentUser   string
		preAuthorized bool
	}{
		{name: "pull request author", commentUser: `"id": 71, "login": "author1"`, preAuthorized: true},
		{name: "pull request assignee", commentUser: `"id": 7101, "login": "alice"`, preAuthorized: true},
		{name: "outside requester", commentUser: `"id": 3, "login": "reviewer1"`, preAuthorized: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := strings.Replace(string(githubCommentPayload(true, "请调整实现。")),
				`"user": {"id": 3, "login": "reviewer1"}`, `"user": {`+testCase.commentUser+`}`, 1)
			result, err := GitHubNormalizer{}.Normalize([]byte(payload))
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

func TestGitHubNormalizerIgnoresPRCommentOnClosedMergeRequest(t *testing.T) {
	payload := strings.Replace(string(githubCommentPayload(true, "请调整实现。")), `"state": "open"`, `"state": "closed"`, 1)
	result, err := GitHubNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.Event != nil {
		t.Fatalf("expected closed merge request comment to be ignored, got %#v", result)
	}
}

// 评论缺作者时不能 panic,也无法通过下游 requester 授权,应显式忽略。
func TestGitHubNormalizerIgnoresPRCommentWithoutAuthor(t *testing.T) {
	payload := strings.Replace(string(githubCommentPayload(true, "请调整实现。")),
		`"user": {"id": 3, "login": "reviewer1"}`, `"user": null`, 1)
	result, err := GitHubNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize panicked or failed: %v", err)
	}
	if !result.Ignored || result.Event != nil {
		t.Fatalf("expected comment without author to be ignored, got %#v", result)
	}
}

func TestGitHubNormalizerMapsIssueCommentToResume(t *testing.T) {
	payload := strings.Replace(string(githubCommentPayload(false, "补充:状态枚举为 todo/doing/done。")),
		`"user": {"id": 3, "login": "reviewer1"}`, `"user": {"id": 71, "login": "author1"}`, 1)
	result, err := GitHubNormalizer{}.Normalize([]byte(payload))
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
	if event.DedupeKey != "github:issue-develop:42:7" {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
}

func TestGitHubNormalizerReceiptPrefersDeliveryID(t *testing.T) {
	receipt := GitHubNormalizer{}.Receipt([]byte(`{}`), WebhookMetadata{EventType: "issues", DeliveryID: "d-1"})
	if receipt.DedupeKey != "github:webhook:delivery:d-1" {
		t.Fatalf("unexpected delivery dedupe key: %s", receipt.DedupeKey)
	}

	receipt = GitHubNormalizer{}.Receipt([]byte(githubIssueOpenedPayload), WebhookMetadata{EventType: "issues"})
	if receipt.DedupeKey == "" || strings.Contains(receipt.DedupeKey, "delivery") {
		t.Fatalf("expected payload-derived dedupe key, got %q", receipt.DedupeKey)
	}
	if receipt.ExternalID != "42:7" {
		t.Fatalf("unexpected external id: %s", receipt.ExternalID)
	}
}

// resume 请求者白名单:作者 ∪ 指派人 ∪ 平台白名单;陌生人触发应被忽略。
func TestGitHubNormalizerIgnoresResumeFromUnauthorizedRequester(t *testing.T) {
	result, err := GitHubNormalizer{}.Normalize(githubCommentPayload(false, "补充:状态枚举为 todo/doing/done。"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.IgnoreReason != "resume requester is not authorized" {
		t.Fatalf("expected unauthorized requester to be ignored, got %#v", result)
	}
}

func TestGitHubNormalizerAllowsResumeForPlatformAllowlist(t *testing.T) {
	normalizer := NewGitHubNormalizer(WithGitHubReworkAllowedUserIDs([]int{3}))
	result, err := normalizer.Normalize(githubCommentPayload(false, "补充:状态枚举为 todo/doing/done。"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event == nil {
		t.Fatalf("expected allowlisted requester resume to be accepted, got %#v", result)
	}
}

// 作者身份不可得且无指派人时 fail-closed 拒绝:「仅作者」边界无法确立的触发
// 不放行(真实载荷作者恒可得,该分支为防御性收口)。
func TestGitHubNormalizerRejectsResumeWhenAuthorUnavailable(t *testing.T) {
	payload := string(githubCommentPayload(false, "补充:状态枚举为 todo/doing/done。"))
	payload = strings.Replace(payload, `"user": {"id": 71, "login": "author1"},`, `"user": null,`, 1)
	payload = strings.Replace(payload, `"assignees": [{"id": 7101, "login": "alice"}], `, `"assignees": [], `, 1)
	result, err := GitHubNormalizer{}.Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.IgnoreReason != "resume requester is not authorized" {
		t.Fatalf("expected unauthorized ignore, got %#v", result)
	}
}
