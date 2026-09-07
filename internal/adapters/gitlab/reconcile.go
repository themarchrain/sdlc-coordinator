package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
)

type reconcileIssue struct {
	IID         int      `json:"iid"`
	WebURL      string   `json:"web_url"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
}

type reconcileMergeRequest struct {
	IID          int      `json:"iid"`
	WebURL       string   `json:"web_url"`
	Description  string   `json:"description"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	Labels       []string `json:"labels"`
}

type reconcileNote struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

func (c *Client) ReconcileWriteback(ctx context.Context, req codehost.ReconcileRequest) (codehost.ReconcileResult, error) {
	projectPath := fmt.Sprintf("/api/v4/projects/%s", url.PathEscape(req.ProjectID))
	switch req.Kind {
	case "create_merge_request":
		query := url.Values{
			"scope":         {"all"},
			"state":         {"all"},
			"source_branch": {req.SourceBranch},
			"target_branch": {req.TargetBranch},
			"per_page":      {"100"},
		}
		var mergeRequests []reconcileMergeRequest
		if err := c.doJSON(ctx, http.MethodGet, projectPath+"/merge_requests?"+query.Encode(), nil, &mergeRequests); err != nil {
			return codehost.ReconcileResult{}, err
		}
		for _, mr := range mergeRequests {
			if mr.SourceBranch == req.SourceBranch && mr.TargetBranch == req.TargetBranch && strings.Contains(mr.Description, req.OperationMarker) {
				return foundReconcileResult(mr.IID, mr.WebURL, mr), nil
			}
		}
	case "create_issue":
		query := url.Values{"scope": {"all"}, "state": {"all"}, "search": {req.DedupeKey}, "in": {"description"}, "per_page": {"100"}}
		var issues []reconcileIssue
		if err := c.doJSON(ctx, http.MethodGet, projectPath+"/issues?"+query.Encode(), nil, &issues); err != nil {
			return codehost.ReconcileResult{}, err
		}
		for _, issue := range issues {
			if strings.Contains(issue.Description, req.OperationMarker) {
				return foundReconcileResult(issue.IID, issue.WebURL, issue), nil
			}
		}
	case "create_issue_note", "create_merge_request_note":
		resource := "issues"
		if req.Kind == "create_merge_request_note" {
			resource = "merge_requests"
		}
		var notes []reconcileNote
		path := fmt.Sprintf("%s/%s/%d/notes?order_by=created_at&per_page=100&sort=desc", projectPath, resource, req.TargetIID)
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &notes); err != nil {
			return codehost.ReconcileResult{}, err
		}
		for _, note := range notes {
			if strings.Contains(note.Body, req.OperationMarker) {
				return foundReconcileResult(note.ID, "", note), nil
			}
		}
	case "set_issue_labels":
		var issue reconcileIssue
		if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/issues/%d", projectPath, req.TargetIID), nil, &issue); err != nil {
			return codehost.ReconcileResult{}, err
		}
		if sameLabels(issue.Labels, req.Labels) {
			return foundReconcileResult(issue.IID, issue.WebURL, issue), nil
		}
	case "set_merge_request_labels", "add_merge_request_labels":
		var mr reconcileMergeRequest
		if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/merge_requests/%d", projectPath, req.TargetIID), nil, &mr); err != nil {
			return codehost.ReconcileResult{}, err
		}
		matched := sameLabels(mr.Labels, req.Labels)
		if req.Kind == "add_merge_request_labels" {
			matched = containsAllLabels(mr.Labels, req.Labels)
		}
		if matched {
			return foundReconcileResult(mr.IID, mr.WebURL, mr), nil
		}
	default:
		return codehost.ReconcileResult{}, fmt.Errorf("unsupported gitlab reconcile kind %q", req.Kind)
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
