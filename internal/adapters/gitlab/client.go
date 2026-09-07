package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func NewClient(baseURL, token string, opts ...Option) *Client {
	client := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: http.DefaultClient,
	}

	for _, opt := range opts {
		opt(client)
	}

	if client.httpClient == nil {
		client.httpClient = http.DefaultClient
	}

	return client
}

type CreateIssueRequest = codehost.CreateIssueRequest
type CreateMergeRequestRequest = codehost.CreateMergeRequestRequest
type CreateNoteRequest = codehost.CreateNoteRequest
type SetLabelsRequest = codehost.SetLabelsRequest
type Issue = codehost.Issue
type MergeRequest = codehost.MergeRequest
type Note = codehost.Note

type Project struct {
	ID                int    `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
	DefaultBranch     string `json:"default_branch"`
}

type StatusError struct {
	Method     string
	Path       string
	Status     string
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	if e == nil {
		return "gitlab status error"
	}
	return fmt.Sprintf("gitlab %s %s: %s: %s", e.Method, e.Path, e.Status, strings.TrimSpace(e.Body))
}

func StatusCode(err error) int {
	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode
	}
	return 0
}

func (c *Client) GetProject(ctx context.Context, pathOrID string) (*Project, error) {
	var project Project
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v4/projects/%s", url.PathEscape(pathOrID)), nil, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) CreateIssue(ctx context.Context, projectID string, req codehost.CreateIssueRequest) (*codehost.Issue, error) {
	payload := map[string]any{
		"title":       req.Title,
		"description": req.Description,
		"labels":      strings.Join(req.Labels, ","),
	}

	var issue codehost.Issue
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v4/projects/%s/issues", url.PathEscape(projectID)), payload, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

func (c *Client) CreateMergeRequest(ctx context.Context, projectID string, req codehost.CreateMergeRequestRequest) (*codehost.MergeRequest, error) {
	payload := map[string]any{
		"title":                req.Title,
		"description":          req.Description,
		"source_branch":        req.SourceBranch,
		"target_branch":        req.TargetBranch,
		"remove_source_branch": req.RemoveSourceBranch,
		"labels":               strings.Join(req.Labels, ","),
	}

	var mr codehost.MergeRequest
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v4/projects/%s/merge_requests", url.PathEscape(projectID)), payload, &mr); err != nil {
		return nil, err
	}
	return &mr, nil
}

func (c *Client) CreateIssueNote(ctx context.Context, projectID string, issueIID int, req codehost.CreateNoteRequest) (*codehost.Note, error) {
	var note codehost.Note
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v4/projects/%s/issues/%d/notes", url.PathEscape(projectID), issueIID), map[string]any{
		"body": req.Body,
	}, &note); err != nil {
		return nil, err
	}
	return &note, nil
}

func (c *Client) CreateMergeRequestNote(ctx context.Context, projectID string, mrIID int, req codehost.CreateNoteRequest) (*codehost.Note, error) {
	var note codehost.Note
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d/notes", url.PathEscape(projectID), mrIID), map[string]any{
		"body": req.Body,
	}, &note); err != nil {
		return nil, err
	}
	return &note, nil
}

func (c *Client) SetIssueLabels(ctx context.Context, projectID string, issueIID int, req codehost.SetLabelsRequest) (*codehost.Issue, error) {
	var issue codehost.Issue
	if err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/api/v4/projects/%s/issues/%d", url.PathEscape(projectID), issueIID), map[string]any{
		"labels": strings.Join(req.Labels, ","),
	}, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

func (c *Client) SetMergeRequestLabels(ctx context.Context, projectID string, mrIID int, req codehost.SetLabelsRequest) (*codehost.MergeRequest, error) {
	var mr codehost.MergeRequest
	if err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d", url.PathEscape(projectID), mrIID), map[string]any{
		"labels": strings.Join(req.Labels, ","),
	}, &mr); err != nil {
		return nil, err
	}
	return &mr, nil
}

func (c *Client) AddMergeRequestLabels(ctx context.Context, projectID string, mrIID int, req codehost.SetLabelsRequest) (*codehost.MergeRequest, error) {
	var mr codehost.MergeRequest
	if err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/api/v4/projects/%s/merge_requests/%d", url.PathEscape(projectID), mrIID), map[string]any{
		"add_labels": strings.Join(req.Labels, ","),
	}, &mr); err != nil {
		return nil, err
	}
	return &mr, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(in); err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		body = buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("PRIVATE-TOKEN", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return codehost.NewAmbiguousError(fmt.Errorf("perform request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		err := fmt.Errorf("gitlab %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
		if resp.StatusCode >= http.StatusInternalServerError {
			return codehost.NewAmbiguousError(err)
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout {
			return codehost.NewTransientRejection(&StatusError{
				Method:     method,
				Path:       path,
				Status:     resp.Status,
				StatusCode: resp.StatusCode,
				Body:       strings.TrimSpace(string(data)),
			})
		}
		if resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError {
			return &StatusError{
				Method:     method,
				Path:       path,
				Status:     resp.Status,
				StatusCode: resp.StatusCode,
				Body:       strings.TrimSpace(string(data)),
			}
		}
		return err
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		err := fmt.Errorf("decode response: %w", err)
		if isMutatingMethod(method) {
			return codehost.NewAmbiguousError(err)
		}
		return err
	}
	return nil
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
