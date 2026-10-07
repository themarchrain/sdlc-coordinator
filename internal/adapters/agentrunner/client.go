package agentrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"sdlc-coordinator/internal/runner"
)

const defaultHTTPTimeout = 30 * time.Second

type Client struct {
	baseURL string
	http    *http.Client
	// internalToken 是受控工作区操作面的部署级令牌（协调器专属）；空值表示
	// 未配置，服务端会以 503 fail-closed。
	internalToken string
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), http: httpClient}
}

func (c *Client) Create(ctx context.Context, request runner.CreateExecutionRequest) (*runner.Execution, error) {
	var execution runner.Execution
	if err := c.do(ctx, http.MethodPost, "/api/v1/executions", request, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

func (c *Client) Get(ctx context.Context, executionID string) (*runner.Execution, error) {
	var execution runner.Execution
	if err := c.do(ctx, http.MethodGet, executionPath(executionID), nil, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

func (c *Client) Events(ctx context.Context, executionID string, after int64) ([]runner.Event, error) {
	var response struct {
		Items []runner.Event `json:"items"`
	}
	path := executionPath(executionID) + "/events?after_seq=" + strconv.FormatInt(after, 10)
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (c *Client) Result(ctx context.Context, executionID string) (*runner.Result, error) {
	var result runner.Result
	if err := c.do(ctx, http.MethodGet, executionPath(executionID)+"/result", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Cancel(ctx context.Context, executionID string) (*runner.Execution, error) {
	var execution runner.Execution
	if err := c.do(ctx, http.MethodPost, executionPath(executionID)+"/cancel", struct{}{}, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

func (c *Client) Delete(ctx context.Context, executionID string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+executionPath(executionID), nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotFound {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	return &runner.HTTPError{Method: http.MethodDelete, Path: executionPath(executionID), StatusCode: response.StatusCode, Body: strings.TrimSpace(string(raw))}
}

func (c *Client) do(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.internalToken != "" {
		request.Header.Set("Authorization", "Bearer "+c.internalToken)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return &runner.HTTPError{Method: method, Path: path, StatusCode: response.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("decode agent runner response: %w", err)
	}
	return nil
}

func executionPath(executionID string) string {
	return "/api/v1/executions/" + url.PathEscape(strings.TrimSpace(executionID))
}
