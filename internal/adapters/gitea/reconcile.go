package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
)

type reconcilePullRequest struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Head    struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

type reconcileIssue struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
}

func (c *Client) ReconcileWriteback(ctx context.Context, req codehost.ReconcileRequest) (codehost.ReconcileResult, error) {
	owner, name, err := splitRepo(req.ProjectID)
	if err != nil {
		return codehost.ReconcileResult{}, err
	}
	repoPath := fmt.Sprintf("/api/v1/repos/%s/%s", url.PathEscape(owner), url.PathEscape(name))
	switch req.Kind {
	case "create_merge_request":
		// 列表接口按页返回(服务器会把更大的 limit 截到自身上限),翻页直到
		// 短页,避免老仓库里目标对象落在第一页之外被误判 absent。
		for page := 1; ; page++ {
			query := url.Values{"state": {"all"}, "page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(giteaPageSize)}}
			var pulls []reconcilePullRequest
			if err := c.doJSON(ctx, http.MethodGet, repoPath+"/pulls?"+query.Encode(), nil, &pulls); err != nil {
				return codehost.ReconcileResult{}, err
			}
			for _, pull := range pulls {
				if pull.Head.Ref == req.SourceBranch && pull.Base.Ref == req.TargetBranch && strings.Contains(pull.Body, req.OperationMarker) {
					return foundReconcileResult(pull.Number, pull.HTMLURL, pull), nil
				}
			}
			if len(pulls) < giteaPageSize {
				break
			}
		}
	case "create_issue":
		for page := 1; ; page++ {
			query := url.Values{"state": {"all"}, "type": {"issues"}, "q": {req.DedupeKey}, "page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(giteaPageSize)}}
			var issues []reconcileIssue
			if err := c.doJSON(ctx, http.MethodGet, repoPath+"/issues?"+query.Encode(), nil, &issues); err != nil {
				return codehost.ReconcileResult{}, err
			}
			for _, issue := range issues {
				if strings.Contains(issue.Body, req.OperationMarker) {
					return foundReconcileResult(issue.Number, issue.HTMLURL, issue), nil
				}
			}
			if len(issues) < giteaPageSize {
				break
			}
		}
	case "create_issue_note", "create_merge_request_note":
		for page := 1; ; page++ {
			query := url.Values{"page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(giteaPageSize)}}
			var comments []noteResponse
			path := fmt.Sprintf("%s/issues/%d/comments?%s", repoPath, req.TargetIID, query.Encode())
			if err := c.doJSON(ctx, http.MethodGet, path, nil, &comments); err != nil {
				return codehost.ReconcileResult{}, err
			}
			for _, comment := range comments {
				if strings.Contains(comment.Body, req.OperationMarker) {
					return foundReconcileResult(comment.ID, "", comment), nil
				}
			}
			if len(comments) < giteaPageSize {
				break
			}
		}
	case "set_issue_labels", "set_merge_request_labels", "add_merge_request_labels":
		var labels []labelResponse
		path := fmt.Sprintf("%s/issues/%d/labels", repoPath, req.TargetIID)
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &labels); err != nil {
			return codehost.ReconcileResult{}, err
		}
		actual := make([]string, 0, len(labels))
		for _, label := range labels {
			actual = append(actual, label.Name)
		}
		matched := sameLabels(actual, req.Labels)
		if req.Kind == "add_merge_request_labels" {
			matched = containsAllLabels(actual, req.Labels)
		}
		if matched {
			return foundReconcileResult(req.TargetIID, "", labels), nil
		}
	default:
		return codehost.ReconcileResult{}, fmt.Errorf("unsupported gitea reconcile kind %q", req.Kind)
	}
	return codehost.ReconcileResult{State: codehost.ReconcileStateAbsent}, nil
}

func containsAllLabels(actual, expected []string) bool {
	present := make(map[string]struct{}, len(actual))
	for _, label := range actual {
		present[label] = struct{}{}
	}
	for _, label := range expected {
		if _, ok := present[label]; !ok {
			return false
		}
	}
	return true
}

func foundReconcileResult(id int, externalURL string, response any) codehost.ReconcileResult {
	return codehost.ReconcileResult{
		State:       codehost.ReconcileStateFound,
		ExternalID:  strconv.Itoa(id),
		ExternalURL: externalURL,
		Response:    response,
	}
}

func sameLabels(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	counts := make(map[string]int, len(actual))
	for _, label := range actual {
		counts[label]++
	}
	for _, label := range expected {
		if counts[label] == 0 {
			return false
		}
		counts[label]--
	}
	return true
}
