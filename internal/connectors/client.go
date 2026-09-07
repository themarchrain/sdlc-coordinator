package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 5 * time.Second
	defaultTriggerTimeout = 5 * time.Minute
	defaultResponseLimit  = int64(1 << 20)
)

type RemoteErrorKind string

const (
	RemoteErrorUnavailable RemoteErrorKind = "unavailable"
	RemoteErrorTimeout     RemoteErrorKind = "timeout"
	RemoteErrorContract    RemoteErrorKind = "contract_error"
	RemoteErrorRemote      RemoteErrorKind = "remote_error"
)

var (
	ErrDuplicateResource      = errors.New("duplicate_resource")
	ErrRepositoryIDMismatch   = errors.New("repository_id_mismatch")
	ErrConnectorNotConfigured = errors.New("configuration_required")
)

type RemoteError struct {
	Kind       RemoteErrorKind
	StatusCode int
	// Code is the connector's machine-readable error code. Only the code is
	// carried over, never the remote message, so error text stays bounded and
	// cannot leak secrets from the connector response body.
	Code string
	Err  error
}

func (err *RemoteError) Error() string {
	code := ""
	if err.Code != "" {
		code = " [" + err.Code + "]"
	}
	if err.StatusCode > 0 {
		return fmt.Sprintf("connector %s (%d)%s: %v", err.Kind, err.StatusCode, code, err.Err)
	}
	return fmt.Sprintf("connector %s%s: %v", err.Kind, code, err.Err)
}

func (err *RemoteError) Unwrap() error {
	return err.Err
}

type HTTPClientOption func(*HTTPClient)

type HTTPClient struct {
	client         *http.Client
	requestTimeout time.Duration
	triggerTimeout time.Duration
	responseLimit  int64
}

func NewHTTPClient(options ...HTTPClientOption) *HTTPClient {
	client := &HTTPClient{
		client: &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		requestTimeout: defaultRequestTimeout,
		triggerTimeout: defaultTriggerTimeout,
		responseLimit:  defaultResponseLimit,
	}
	for _, option := range options {
		option(client)
	}
	return client
}

func WithRequestTimeouts(requestTimeout, triggerTimeout time.Duration) HTTPClientOption {
	return func(client *HTTPClient) {
		if requestTimeout > 0 {
			client.requestTimeout = requestTimeout
		}
		if triggerTimeout > 0 {
			client.triggerTimeout = triggerTimeout
		}
	}
}

func WithResponseLimit(limit int64) HTTPClientOption {
	return func(client *HTTPClient) {
		if limit > 0 {
			client.responseLimit = limit
		}
	}
}

func (client *HTTPClient) Manifest(ctx context.Context, baseURL string) (Manifest, error) {
	var result Manifest
	err := client.doJSON(ctx, http.MethodGet, baseURL, "/plugin/v1/manifest", nil, nil, &result, client.requestTimeout)
	return result, err
}

func (client *HTTPClient) Health(ctx context.Context, baseURL string) (Health, error) {
	var result Health
	err := client.doJSON(ctx, http.MethodGet, baseURL, "/plugin/v1/health", nil, nil, &result, client.requestTimeout)
	return result, err
}

func (client *HTTPClient) TriggerRun(ctx context.Context, baseURL, requestID, repositoryID string) (Run, error) {
	body := map[string]string{"request_id": requestID}
	if id := strings.TrimSpace(repositoryID); id != "" {
		body["repository_id"] = id
	}
	var result Run
	err := client.doJSON(
		ctx, http.MethodPost, baseURL, "/plugin/v1/runs", nil,
		body, &result, client.triggerTimeout,
	)
	return result, err
}

func (client *HTTPClient) ListRuns(ctx context.Context, baseURL string, pageRequest PageRequest) (RemotePage[Run], error) {
	var result RemotePage[Run]
	err := client.doJSON(ctx, http.MethodGet, baseURL, "/plugin/v1/runs", pageQuery(pageRequest), nil, &result, client.requestTimeout)
	return result, err
}

func (client *HTTPClient) GetRun(ctx context.Context, baseURL, runID string) (Run, error) {
	var result Run
	err := client.doJSON(ctx, http.MethodGet, baseURL, "/plugin/v1/runs/"+url.PathEscape(runID), nil, nil, &result, client.requestTimeout)
	return result, err
}

func (client *HTTPClient) ListRecords(ctx context.Context, baseURL string, pageRequest PageRequest) (RemotePage[Record], error) {
	var result RemotePage[Record]
	err := client.doJSON(ctx, http.MethodGet, baseURL, "/plugin/v1/records", pageQuery(pageRequest), nil, &result, client.requestTimeout)
	return result, err
}

func (client *HTTPClient) GetRecord(ctx context.Context, baseURL, recordID string) (Record, error) {
	var result Record
	err := client.doJSON(ctx, http.MethodGet, baseURL, "/plugin/v1/records/"+url.PathEscape(recordID), nil, nil, &result, client.requestTimeout)
	return result, err
}

func (client *HTTPClient) Metrics(ctx context.Context, baseURL, repositoryID string) (Metrics, error) {
	var result Metrics
	err := client.doJSON(ctx, http.MethodGet, baseURL, "/plugin/v1/metrics", repositoryQuery(repositoryID), nil, &result, client.requestTimeout)
	return result, err
}

func (client *HTTPClient) Configuration(ctx context.Context, baseURL string) (Configuration, error) {
	var result Configuration
	err := client.doJSON(ctx, http.MethodGet, baseURL, "/plugin/v1/configuration", nil, nil, &result, client.requestTimeout)
	return result, err
}

func (client *HTTPClient) SaveConfiguration(ctx context.Context, baseURL string, request SaveConfigurationRequest) (Configuration, error) {
	var result Configuration
	err := client.doJSON(ctx, http.MethodPut, baseURL, "/plugin/v1/configuration", nil, request, &result, client.requestTimeout)
	return result, err
}

func (client *HTTPClient) PutRepository(ctx context.Context, baseURL, repositoryID string, request SaveRepositoryRequest) (RemoteRepository, error) {
	repositoryID = strings.TrimSpace(repositoryID)
	if strings.TrimSpace(request.RepositoryID) == "" {
		request.RepositoryID = repositoryID
	}
	if request.RepositoryID != repositoryID {
		return RemoteRepository{}, &RemoteError{Kind: RemoteErrorContract, StatusCode: http.StatusBadRequest, Err: ErrRepositoryIDMismatch}
	}
	var result RemoteRepository
	err := client.doJSON(
		ctx, http.MethodPut, baseURL, "/plugin/v1/repositories/"+url.PathEscape(repositoryID),
		nil, request, &result, client.requestTimeout,
	)
	if err != nil {
		return RemoteRepository{}, mapRepositoryRemoteError(err)
	}
	return result, nil
}

func (client *HTTPClient) GetRepository(ctx context.Context, baseURL, repositoryID string) (RemoteRepository, error) {
	var result RemoteRepository
	err := client.doJSON(
		ctx, http.MethodGet, baseURL, "/plugin/v1/repositories/"+url.PathEscape(repositoryID),
		nil, nil, &result, client.requestTimeout,
	)
	if err != nil {
		return RemoteRepository{}, err
	}
	return result, nil
}

func (client *HTTPClient) DeleteRepository(ctx context.Context, baseURL, repositoryID string) error {
	return client.doJSON(
		ctx, http.MethodDelete, baseURL, "/plugin/v1/repositories/"+url.PathEscape(repositoryID),
		nil, nil, nil, client.requestTimeout,
	)
}

// Reload asks the connector to exit so its orchestrator restarts it with the
// latest database configuration. The connector treats this as idempotent.
func (client *HTTPClient) Reload(ctx context.Context, baseURL string) error {
	return client.doJSON(
		ctx, http.MethodPost, baseURL, "/plugin/v1/reload",
		nil, nil, nil, client.requestTimeout,
	)
}

// remoteErrorCode extracts the connector's `{"error":{"code":...}}` token and
// drops anything that is not a plain snake_case identifier, so an arbitrary
// remote body can never widen the error text.
func remoteErrorCode(payload []byte) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ""
	}
	code := strings.TrimSpace(envelope.Error.Code)
	if code == "" || len(code) > 48 {
		return ""
	}
	for _, r := range code {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return ""
		}
	}
	return code
}

func mapRepositoryRemoteError(err error) error {
	var remoteError *RemoteError
	if errors.As(err, &remoteError) {
		if mapped := repositoryRemoteCause(remoteError); mapped != nil {
			err = &RemoteError{Kind: remoteError.Kind, StatusCode: remoteError.StatusCode, Code: remoteError.Code, Err: mapped}
		}
	}
	return err
}

func repositoryRemoteCause(remoteError *RemoteError) error {
	switch remoteError.Code {
	case "configuration_required":
		return ErrConnectorNotConfigured
	case "duplicate_resource":
		return ErrDuplicateResource
	}
	return nil
}

func (client *HTTPClient) doJSON(
	ctx context.Context,
	method, baseURL, endpoint string,
	query url.Values,
	body any,
	target any,
	timeout time.Duration,
) error {
	requestURL, err := connectorURL(baseURL, endpoint, query)
	if err != nil {
		return &RemoteError{Kind: RemoteErrorContract, Err: err}
	}

	var requestBody io.Reader
	if body != nil {
		encoded, encodeErr := json.Marshal(body)
		if encodeErr != nil {
			return &RemoteError{Kind: RemoteErrorContract, Err: fmt.Errorf("encode request: %w", encodeErr)}
		}
		requestBody = bytes.NewReader(encoded)
	}

	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, method, requestURL, requestBody)
	if err != nil {
		return &RemoteError{Kind: RemoteErrorContract, Err: fmt.Errorf("build request: %w", err)}
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return &RemoteError{Kind: RemoteErrorTimeout, Err: err}
		}
		return &RemoteError{Kind: RemoteErrorUnavailable, Err: err}
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(response.Body, client.responseLimit+1))
	if err != nil {
		return &RemoteError{Kind: RemoteErrorUnavailable, StatusCode: response.StatusCode, Err: fmt.Errorf("read response: %w", err)}
	}
	if int64(len(payload)) > client.responseLimit {
		return &RemoteError{Kind: RemoteErrorContract, StatusCode: response.StatusCode, Err: errors.New("response exceeds size limit")}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := http.StatusText(response.StatusCode)
		if message == "" {
			message = "remote request failed"
		}
		return &RemoteError{Kind: RemoteErrorRemote, StatusCode: response.StatusCode, Code: remoteErrorCode(payload), Err: errors.New(message)}
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return &RemoteError{Kind: RemoteErrorContract, StatusCode: response.StatusCode, Err: fmt.Errorf("decode response: %w", err)}
	}
	return nil
}

func connectorURL(baseURL, endpoint string, query url.Values) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("base url scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", errors.New("base url host is required")
	}
	parsed.Path = path.Join(strings.TrimSuffix(parsed.Path, "/"), endpoint)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func pageQuery(request PageRequest) url.Values {
	normalized := request.normalize()
	values := url.Values{
		"page":      []string{strconv.Itoa(normalized.Page)},
		"page_size": []string{strconv.Itoa(normalized.PageSize)},
	}
	if id := strings.TrimSpace(normalized.RepositoryID); id != "" {
		values.Set("repository_id", id)
	}
	return values
}

func repositoryQuery(repositoryID string) url.Values {
	id := strings.TrimSpace(repositoryID)
	if id == "" {
		return nil
	}
	return url.Values{"repository_id": []string{id}}
}
