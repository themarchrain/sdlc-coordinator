package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sdlc-coordinator/internal/workspaceproto"
)

func NewHTTPAuthorizer(endpointURL, token string, httpClient *http.Client) func(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
	url := strings.TrimRight(strings.TrimSpace(endpointURL), "/")
	token = strings.TrimSpace(token)
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return func(ctx context.Context, request workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
		if url == "" {
			return workspaceproto.AuthorizationDecision{}, fmt.Errorf("%w: authorization endpoint is not configured", ErrNotAuthorized)
		}
		raw, err := json.Marshal(request)
		if err != nil {
			return workspaceproto.AuthorizationDecision{}, fmt.Errorf("encode authorization request: %w", err)
		}
		httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return workspaceproto.AuthorizationDecision{}, err
		}
		httpRequest.Header.Set("Content-Type", "application/json")
		if token != "" {
			httpRequest.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := httpClient.Do(httpRequest)
		if err != nil {
			return workspaceproto.AuthorizationDecision{}, fmt.Errorf("confirm workspace operation authorization: %w", err)
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
			return workspaceproto.AuthorizationDecision{}, fmt.Errorf("confirm workspace operation authorization: http %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
		}
		var decision workspaceproto.AuthorizationDecision
		if err := json.NewDecoder(io.LimitReader(response.Body, 8<<10)).Decode(&decision); err != nil {
			return workspaceproto.AuthorizationDecision{}, fmt.Errorf("decode authorization decision: %w", err)
		}
		return decision, nil
	}
}
