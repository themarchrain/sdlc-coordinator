package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
)

type reconcilePullRequest struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Head    struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
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

type reconcileLabel struct {
	Name string `json:"name"`
}

const (
	reconcilePerPage = 100
	reconcileMaxPage = 50
)

// ReconcileWriteback 对账写回意图;GitHub 列表接口分页(per_page 上限 100),
// 翻页直到短页,避免老仓库目标对象落在第一页之外被误判 absent。
func (c *Client) ReconcileWriteback(ctx context.Context, req codehost.ReconcileRequest) (codehost.ReconcileResult, error) {
	owner, name, err := splitRepo(req.ProjectID)
	if err != nil {
		return codehost.ReconcileResult{}, err
	}
	repoPath := fmt.Sprintf("/repos/%s/%s", owner, name)
	switch req.Kind {
	case "create_merge_request":
		var found *codehost.ReconcileResult
		err = c.listAll(ctx, repoPath+"/pulls?state=all", reconcilePerPage, reconcileMaxPage, func(raw json.RawMessage) (bool, error) {
			var pulls []reconcilePullRequest
			if err := json.Unmarshal(raw, &pulls); err != nil {
				return false, err
			}
			for _, pull := range pulls {
				if pull.Head.Ref == req.SourceBranch && pull.Base.Ref == req.TargetBranch && strings.Contains(pull.Body, req.OperationMarker) {
					result := foundReconcileResult(pull.Number, pull.HTMLURL, pull)
					found = &result
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return codehost.ReconcileResult{}, err
		}
		if found != nil {
			return *found, nil
		}
	case "create_issue":
		// GitHub issues 列表同时返回 PR(PR 也是 issue),operation marker 保证匹配精确。
		var found *codehost.ReconcileResult
		err = c.listAll(ctx, repoPath+"/issues?state=all", reconcilePerPage, reconcileMaxPage, func(raw json.RawMessage) (bool, error) {
			var issues []reconcileIssue
			if err := json.Unmarshal(raw, &issues); err != nil {
				return false, err
			}
			for _, issue := range issues {
				if strings.Contains(issue.Body, req.OperationMarker) {
					result := foundReconcileResult(issue.Number, issue.HTMLURL, issue)
					found = &result
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return codehost.ReconcileResult{}, err
		}
		if found != nil {
			return *found, nil
		}
	case "create_issue_note", "create_merge_request_note":
		var found *codehost.ReconcileResult
		err = c.listAll(ctx, fmt.Sprintf("%s/issues/%d/comments", repoPath, req.TargetIID), reconcilePerPage, reconcileMaxPage, func(raw json.RawMessage) (bool, error) {
			var notes []noteResponse
			if err := json.Unmarshal(raw, &notes); err != nil {
				return false, err
			}
			for _, note := range notes {
				if strings.Contains(note.Body, req.OperationMarker) {
					result := foundReconcileResult(note.ID, "", note)
					found = &result
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return codehost.ReconcileResult{}, err
		}
		if found != nil {
			return *found, nil
		}
	case "set_issue_labels", "set_merge_request_labels", "add_merge_request_labels":
		var actual []string
		err = c.listAll(ctx, fmt.Sprintf("%s/issues/%d/labels", repoPath, req.TargetIID), reconcilePerPage, reconcileMaxPage, func(raw json.RawMessage) (bool, error) {
			var labels []reconcileLabel
			if err := json.Unmarshal(raw, &labels); err != nil {
				return false, err
			}
			for _, label := range labels {
				actual = append(actual, label.Name)
			}
			return false, nil
		})
		if err != nil {
			return codehost.ReconcileResult{}, err
		}
		matched := sameLabels(actual, req.Labels)
		if req.Kind == "add_merge_request_labels" {
			matched = containsAllLabels(actual, req.Labels)
		}
		if matched {
			return foundReconcileResult(req.TargetIID, "", req.Labels), nil
		}
	default:
		return codehost.ReconcileResult{}, fmt.Errorf("unsupported github reconcile kind %q", req.Kind)
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
		ExternalID:  fmt.Sprintf("%d", id),
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
