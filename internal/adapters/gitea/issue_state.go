package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
)

type giteaIssueState struct {
	State string `json:"state"`
}

func (c *Client) GetIssueState(ctx context.Context, repo string, index int) (codehost.IssueState, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return codehost.IssueState{}, err
	}
	if index <= 0 {
		return codehost.IssueState{}, fmt.Errorf("gitea issue index is required")
	}
	path := fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d", url.PathEscape(owner), url.PathEscape(name), index)
	var payload giteaIssueState
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &payload); err != nil {
		return codehost.IssueState{}, err
	}
	return codehost.IssueState{Closed: strings.EqualFold(strings.TrimSpace(payload.State), "closed")}, nil
}
