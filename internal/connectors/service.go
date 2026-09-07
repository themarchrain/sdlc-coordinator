package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
)

type EnabledHook func(ctx context.Context, connectorID string, enabled bool) error

var (
	ErrInvalidConnector  = errors.New("invalid connector")
	ErrConnectorDisabled = errors.New("connector disabled")
	ErrConnectorContract = errors.New("connector contract mismatch")
	ErrConnectorChanged  = errors.New("connector changed while operation was running")
)

type CreateConnectorRequest struct {
	Kind              string
	DisplayName       string
	BaseURL           string
	Enabled           bool
	TargetProjectPath string
}

type UpdateConnectorRequest struct {
	Kind              *string
	DisplayName       *string
	BaseURL           *string
	Enabled           *bool
	TargetProjectPath *string
}

type ProbeResult struct {
	Connector model.ConnectorInstance
	Manifest  Manifest
	Health    Health
}

type TriggerResult struct {
	RequestID string
	Run       Run
}

type ServiceOption func(*Service)

type Service struct {
	repository   *ConnectorRepository
	remote       RemoteClient
	now          func() time.Time
	newRequestID func() string
	enabledHook  EnabledHook
}

func NewService(repository *ConnectorRepository, remote RemoteClient, options ...ServiceOption) *Service {
	service := &Service{
		repository: repository,
		remote:     remote,
		now:        func() time.Time { return time.Now().UTC() },
		newRequestID: func() string {
			return model.NewID("connector_request")
		},
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func WithClock(now func() time.Time) ServiceOption {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}

func WithRequestIDGenerator(generator func() string) ServiceOption {
	return func(service *Service) {
		if generator != nil {
			service.newRequestID = generator
		}
	}
}

func WithEnabledHook(hook EnabledHook) ServiceOption {
	return func(service *Service) {
		service.enabledHook = hook
	}
}

func (service *Service) SetEnabledHook(hook EnabledHook) {
	if service != nil {
		service.enabledHook = hook
	}
}

func (service *Service) Create(ctx context.Context, request CreateConnectorRequest) (*model.ConnectorInstance, error) {
	normalized, err := normalizeCreateRequest(request)
	if err != nil {
		return nil, err
	}
	connector := &model.ConnectorInstance{
		Kind: normalized.Kind, DisplayName: normalized.DisplayName, BaseURL: normalized.BaseURL,
		Enabled: normalized.Enabled, TargetProjectPath: normalized.TargetProjectPath,
	}
	if err := service.repository.Create(ctx, connector); err != nil {
		return nil, err
	}
	return connector, nil
}

func (service *Service) Get(ctx context.Context, connectorID string) (*model.ConnectorInstance, error) {
	return service.repository.Get(ctx, connectorID)
}

func (service *Service) List(ctx context.Context, filter ConnectorFilter) (Page[model.ConnectorInstance], error) {
	return service.repository.List(ctx, filter)
}

func (service *Service) Update(ctx context.Context, connectorID string, request UpdateConnectorRequest) (*model.ConnectorInstance, error) {
	connector, err := service.repository.Get(ctx, connectorID)
	if err != nil {
		return nil, err
	}
	wasEnabled := connector.Enabled
	if request.Kind != nil {
		connector.Kind = strings.TrimSpace(*request.Kind)
	}
	if request.DisplayName != nil {
		connector.DisplayName = strings.TrimSpace(*request.DisplayName)
	}
	if request.BaseURL != nil {
		connector.BaseURL = *request.BaseURL
	}
	if request.Enabled != nil {
		connector.Enabled = *request.Enabled
	}
	if request.TargetProjectPath != nil {
		connector.TargetProjectPath = strings.TrimSpace(*request.TargetProjectPath)
	}

	normalized, err := normalizeCreateRequest(CreateConnectorRequest{
		Kind: connector.Kind, DisplayName: connector.DisplayName, BaseURL: connector.BaseURL,
		Enabled: connector.Enabled, TargetProjectPath: connector.TargetProjectPath,
	})
	if err != nil {
		return nil, err
	}
	connector.Kind = normalized.Kind
	connector.DisplayName = normalized.DisplayName
	connector.BaseURL = normalized.BaseURL
	connector.TargetProjectPath = normalized.TargetProjectPath
	if err := service.repository.Update(ctx, connector); err != nil {
		return nil, err
	}
	if service.enabledHook != nil && wasEnabled != connector.Enabled {
		if hookErr := service.enabledHook(ctx, connector.ConnectorID, connector.Enabled); hookErr != nil {
			return nil, hookErr
		}
	}
	return connector, nil
}

func (service *Service) Delete(ctx context.Context, connectorID string) error {
	return service.repository.Delete(ctx, connectorID)
}

func (service *Service) Probe(ctx context.Context, connectorID string) (ProbeResult, error) {
	connector, err := service.loadEnabled(ctx, connectorID)
	if err != nil {
		return ProbeResult{}, err
	}

	manifest, err := service.remote.Manifest(ctx, connector.BaseURL)
	if err != nil {
		if persistErr := service.persistHealth(ctx, connector, model.ConnectorHealthUnavailable, probeFailureSummary(err), nil); persistErr != nil {
			return ProbeResult{}, persistErr
		}
		return ProbeResult{}, err
	}
	if manifest.ContractVersion != "1.0" || strings.TrimSpace(manifest.Kind) != connector.Kind {
		message := fmt.Sprintf("expected contract 1.0 kind %s, got contract %s kind %s", connector.Kind, manifest.ContractVersion, manifest.Kind)
		if persistErr := service.persistHealth(ctx, connector, model.ConnectorHealthDegraded, message, manifest.Capabilities); persistErr != nil {
			return ProbeResult{}, persistErr
		}
		return ProbeResult{}, fmt.Errorf("%w: %s", ErrConnectorContract, message)
	}

	health, err := service.remote.Health(ctx, connector.BaseURL)
	if err != nil {
		if persistErr := service.persistHealth(ctx, connector, model.ConnectorHealthUnavailable, probeFailureSummary(err), manifest.Capabilities); persistErr != nil {
			return ProbeResult{}, persistErr
		}
		return ProbeResult{}, err
	}
	healthStatus, valid := connectorHealthStatus(health.Status)
	if !valid {
		message := "unsupported health status " + strings.TrimSpace(health.Status)
		if persistErr := service.persistHealth(ctx, connector, model.ConnectorHealthDegraded, message, manifest.Capabilities); persistErr != nil {
			return ProbeResult{}, persistErr
		}
		return ProbeResult{}, fmt.Errorf("%w: %s", ErrConnectorContract, message)
	}
	if err := service.persistHealth(ctx, connector, healthStatus, health.Message, manifest.Capabilities); err != nil {
		return ProbeResult{}, err
	}
	return ProbeResult{Connector: *connector, Manifest: manifest, Health: health}, nil
}

func (service *Service) TriggerRun(ctx context.Context, connectorID, repositoryID string) (TriggerResult, error) {
	connector, err := service.loadEnabled(ctx, connectorID)
	if err != nil {
		return TriggerResult{}, err
	}
	requestID := service.newRequestID()
	run, err := service.remote.TriggerRun(ctx, connector.BaseURL, requestID, repositoryID)
	if err != nil {
		return TriggerResult{}, err
	}
	return TriggerResult{RequestID: requestID, Run: run}, nil
}

func (service *Service) ListRuns(ctx context.Context, connectorID string, page PageRequest) (RemotePage[Run], error) {
	connector, err := service.loadEnabled(ctx, connectorID)
	if err != nil {
		return RemotePage[Run]{}, err
	}
	return service.remote.ListRuns(ctx, connector.BaseURL, page)
}

func (service *Service) GetRun(ctx context.Context, connectorID, runID string) (Run, error) {
	connector, err := service.loadEnabled(ctx, connectorID)
	if err != nil {
		return Run{}, err
	}
	return service.remote.GetRun(ctx, connector.BaseURL, runID)
}

func (service *Service) ListRecords(ctx context.Context, connectorID string, page PageRequest) (RemotePage[Record], error) {
	connector, err := service.loadEnabled(ctx, connectorID)
	if err != nil {
		return RemotePage[Record]{}, err
	}
	return service.remote.ListRecords(ctx, connector.BaseURL, page)
}

func (service *Service) GetRecord(ctx context.Context, connectorID, recordID string) (Record, error) {
	connector, err := service.loadEnabled(ctx, connectorID)
	if err != nil {
		return Record{}, err
	}
	return service.remote.GetRecord(ctx, connector.BaseURL, recordID)
}

func (service *Service) Metrics(ctx context.Context, connectorID, repositoryID string) (Metrics, error) {
	connector, err := service.loadEnabled(ctx, connectorID)
	if err != nil {
		return Metrics{}, err
	}
	return service.remote.Metrics(ctx, connector.BaseURL, repositoryID)
}

func (service *Service) Configuration(ctx context.Context, connectorID string) (Configuration, error) {
	connector, err := service.loadEnabled(ctx, connectorID)
	if err != nil {
		return Configuration{}, err
	}
	return service.remote.Configuration(ctx, connector.BaseURL)
}

func (service *Service) SaveConfiguration(ctx context.Context, connectorID string, request SaveConfigurationRequest) (Configuration, error) {
	connector, err := service.loadEnabled(ctx, connectorID)
	if err != nil {
		return Configuration{}, err
	}
	return service.remote.SaveConfiguration(ctx, connector.BaseURL, request)
}

func (service *Service) loadEnabled(ctx context.Context, connectorID string) (*model.ConnectorInstance, error) {
	connector, err := service.repository.Get(ctx, connectorID)
	if err != nil {
		return nil, err
	}
	if !connector.Enabled {
		return nil, ErrConnectorDisabled
	}
	return connector, nil
}

func (service *Service) persistHealth(ctx context.Context, connector *model.ConnectorInstance, status model.ConnectorHealthStatus, message string, capabilities []string) error {
	var capabilitiesJSON *string
	if capabilities != nil {
		encoded, err := json.Marshal(capabilities)
		if err != nil {
			return fmt.Errorf("encode connector capabilities: %w", err)
		}
		value := string(encoded)
		capabilitiesJSON = &value
	}
	now := service.now()
	boundedMessage := boundedHealthMessage(message)
	if err := service.repository.UpdateHealthSnapshot(ctx, connector.ConnectorID, connector.BaseURL, HealthSnapshot{
		Status: status, Message: boundedMessage, At: now, CapabilitiesJSON: capabilitiesJSON,
	}); err != nil {
		return err
	}
	if capabilitiesJSON != nil {
		connector.CapabilitiesJSON = *capabilitiesJSON
	}
	connector.LastHealthStatus = status
	connector.LastHealthMessage = boundedMessage
	connector.LastHealthAt = &now
	return nil
}

func probeFailureSummary(err error) string {
	var remoteError *RemoteError
	if !errors.As(err, &remoteError) {
		return "connector probe failed"
	}
	switch remoteError.Kind {
	case RemoteErrorTimeout:
		return "connector request timed out"
	case RemoteErrorUnavailable:
		return "connector unavailable"
	case RemoteErrorContract:
		return "connector contract error"
	case RemoteErrorRemote:
		if remoteError.StatusCode > 0 {
			return fmt.Sprintf("connector remote error (%d)", remoteError.StatusCode)
		}
		return "connector remote error"
	default:
		return "connector probe failed"
	}
}

func boundedHealthMessage(message string) string {
	const maximumRunes = 512
	message = strings.Join(strings.Fields(message), " ")
	runes := []rune(message)
	if len(runes) > maximumRunes {
		runes = runes[:maximumRunes]
	}
	return string(runes)
}

func normalizeCreateRequest(request CreateConnectorRequest) (CreateConnectorRequest, error) {
	request.Kind = strings.TrimSpace(request.Kind)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.TargetProjectPath = strings.TrimSpace(request.TargetProjectPath)
	if request.Kind == "" {
		return CreateConnectorRequest{}, fmt.Errorf("%w: kind is required", ErrInvalidConnector)
	}
	if request.DisplayName == "" {
		return CreateConnectorRequest{}, fmt.Errorf("%w: display_name is required", ErrInvalidConnector)
	}
	normalizedURL, err := normalizeBaseURL(request.BaseURL)
	if err != nil {
		return CreateConnectorRequest{}, fmt.Errorf("%w: %v", ErrInvalidConnector, err)
	}
	request.BaseURL = normalizedURL
	return request, nil
}

func normalizeBaseURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("parse base_url: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("base_url scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", errors.New("base_url host is required")
	}
	if parsed.User != nil {
		return "", errors.New("base_url must not contain userinfo")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", errors.New("base_url must not contain query")
	}
	if parsed.Fragment != "" {
		return "", errors.New("base_url must not contain fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func connectorHealthStatus(raw string) (model.ConnectorHealthStatus, bool) {
	switch model.ConnectorHealthStatus(strings.TrimSpace(raw)) {
	case model.ConnectorHealthHealthy:
		return model.ConnectorHealthHealthy, true
	case model.ConnectorHealthDegraded:
		return model.ConnectorHealthDegraded, true
	case model.ConnectorHealthUnavailable:
		return model.ConnectorHealthUnavailable, true
	default:
		return model.ConnectorHealthUnknown, false
	}
}
