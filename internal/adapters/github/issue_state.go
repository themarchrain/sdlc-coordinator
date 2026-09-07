package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
)

type githubIssueState struct {
	State string `json:"state"`
}

func (c *Client) GetIssueState(ctx context.Context, repo string, number int) (codehost.IssueState, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return codehost.IssueState{}, err
	}
	if number <= 0 {
		return codehost.IssueState{}, fmt.Errorf("github issue number is required")
	}
	path := fmt.Sprintf("/repos/%s/%s/issues/%d", url.PathEscape(owner), url.PathEscape(name), number)
	var payload githubIssueState
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &payload); err != nil {
		return codehost.IssueState{}, err
	}
	return codehost.IssueState{Closed: strings.EqualFold(strings.TrimSpace(payload.State), "closed")}, nil
}
