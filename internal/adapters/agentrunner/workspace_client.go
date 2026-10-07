package agentrunner

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sdlc-coordinator/internal/workspaceproto"
)

// workspaceOperationTimeout 是受控工作区操作的默认 HTTP 超时：clone/commit/
// push 是分钟级操作，比执行面轮询的 30s 宽松。
const workspaceOperationTimeout = 3 * time.Minute

// NewWorkspaceClient 返回受控工作区操作面客户端：部署级内部令牌随请求头
// 发送（协调器专属，不进入 Agent 环境），超时按分钟级 Git 操作设定。
func NewWorkspaceClient(baseURL, internalToken string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: workspaceOperationTimeout}
	}
	client := NewClient(baseURL, httpClient)
	client.internalToken = strings.TrimSpace(internalToken)
	return client
}

func (c *Client) PrepareWorkspace(ctx context.Context, request workspaceproto.PrepareRequest) (*workspaceproto.PrepareResult, error) {
	var result workspaceproto.PrepareResult
	if err := c.do(ctx, http.MethodPost, "/internal/workspace/prepare", request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CommitWorkspace(ctx context.Context, request workspaceproto.CommitRequest) (*workspaceproto.CommitResult, error) {
	var result workspaceproto.CommitResult
	if err := c.do(ctx, http.MethodPost, "/internal/workspace/commit", request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) PushWorkspace(ctx context.Context, request workspaceproto.PushRequest) (*workspaceproto.PushResult, error) {
	var result workspaceproto.PushResult
	if err := c.do(ctx, http.MethodPost, "/internal/workspace/push", request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CleanupWorkspace(ctx context.Context, request workspaceproto.CleanupRequest) (*workspaceproto.CleanupResult, error) {
	var result workspaceproto.CleanupResult
	if err := c.do(ctx, http.MethodPost, "/internal/workspace/cleanup", request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WorkspaceOperation 回读操作记录（固定操作 ID 对账/重放）。
func (c *Client) WorkspaceOperation(ctx context.Context, operationID string) (*workspaceproto.OperationRecord, error) {
	var record workspaceproto.OperationRecord
	path := "/internal/operations/" + url.PathEscape(strings.TrimSpace(operationID))
	if err := c.do(ctx, http.MethodGet, path, nil, &record); err != nil {
		return nil, err
	}
	return &record, nil
}
