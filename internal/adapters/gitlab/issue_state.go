package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
)

type gitlabIssueState struct {
	State string `json:"state"`
}

func (c *Client) GetIssueState(ctx context.Context, projectID string, iid int) (codehost.IssueState, error) {
	if strings.TrimSpace(projectID) == "" {
		return codehost.IssueState{}, fmt.Errorf("gitlab project id is required")
	}
	if iid <= 0 {
		return codehost.IssueState{}, fmt.Errorf("gitlab issue iid is required")
	}
	path := fmt.Sprintf("/api/v4/projects/%s/issues/%d", url.PathEscape(projectID), iid)
	var payload gitlabIssueState
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &payload); err != nil {
		return codehost.IssueState{}, err
	}
	return codehost.IssueState{Closed: strings.EqualFold(strings.TrimSpace(payload.State), "closed")}, nil
}
