package connectors

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
)

func TestServiceValidatesNormalizesAndGatesDisabledConnectors(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	remote := &fakeRemoteClient{}
	service := NewService(repository, remote)
	ctx := context.Background()

	invalidURLs := []string{
		"ftp://sync:8989",
		"http://user:password@sync:8989",
		"http://sync:8989?token=secret",
		"http://sync:8989#fragment",
	}
	for _, baseURL := range invalidURLs {
		_, err := service.Create(ctx, CreateConnectorRequest{Kind: "state_source_sync", DisplayName: "State Source", BaseURL: baseURL, Enabled: true})
		if !errors.Is(err, ErrInvalidConnector) {
			t.Fatalf("expected invalid connector for %q, got %v", baseURL, err)
		}
	}

	connector, err := service.Create(ctx, CreateConnectorRequest{
		Kind: "state_source_sync", DisplayName: "State Source", BaseURL: "HTTP://SYNC:8989/base/", Enabled: false,
	})
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	if connector.BaseURL != "http://sync:8989/base" {
		t.Fatalf("unexpected normalized url %q", connector.BaseURL)
	}
	if _, err := service.ListRuns(ctx, connector.ConnectorID, PageRequest{}); !errors.Is(err, ErrConnectorDisabled) {
		t.Fatalf("expected disabled error, got %v", err)
	}
	if remote.listRunsCalls != 0 {
		t.Fatalf("disabled connector reached remote: %d", remote.listRunsCalls)
	}
}

func TestServiceProbePersistsManifestAndHealthSnapshot(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	now := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	remote := &fakeRemoteClient{
		manifest: Manifest{ContractVersion: "1.0", Kind: "state_source_sync", Name: "State Source Sync", Version: "0.1.0", Capabilities: []string{"manual_trigger", "run_history"}},
		health:   Health{Status: "healthy", Message: "ready"},
	}
	service := NewService(repository, remote, WithClock(func() time.Time { return now }))
	connector, err := service.Create(context.Background(), CreateConnectorRequest{
		Kind: "state_source_sync", DisplayName: "State Source", BaseURL: "http://sync:8989", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}

	probe, err := service.Probe(context.Background(), connector.ConnectorID)
	if err != nil {
		t.Fatalf("probe connector: %v", err)
	}
	if probe.Manifest.Kind != connector.Kind || probe.Health.Status != "healthy" {
		t.Fatalf("unexpected probe: %#v", probe)
	}
	loaded, err := repository.Get(context.Background(), connector.ConnectorID)
	if err != nil {
		t.Fatalf("reload connector: %v", err)
	}
	if loaded.LastHealthStatus != model.ConnectorHealthHealthy || loaded.LastHealthAt == nil || !loaded.LastHealthAt.Equal(now) {
		t.Fatalf("unexpected health snapshot: %#v", loaded)
	}
	if loaded.CapabilitiesJSON != `["manual_trigger","run_history"]` {
		t.Fatalf("unexpected capabilities: %s", loaded.CapabilitiesJSON)
	}
}

func TestServiceProbePersistsUnavailableSnapshot(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	remoteFailure := &RemoteError{Kind: RemoteErrorUnavailable, Err: errors.New("connection refused")}
	remote := &fakeRemoteClient{manifestErr: remoteFailure}
	service := NewService(repository, remote)
	connector, err := service.Create(context.Background(), CreateConnectorRequest{
		Kind: "state_source_sync", DisplayName: "State Source", BaseURL: "http://sync:8989", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}

	if _, err := service.Probe(context.Background(), connector.ConnectorID); !errors.Is(err, remoteFailure) {
		t.Fatalf("expected remote failure, got %v", err)
	}
	loaded, err := repository.Get(context.Background(), connector.ConnectorID)
	if err != nil {
		t.Fatalf("reload connector: %v", err)
	}
	if loaded.LastHealthStatus != model.ConnectorHealthUnavailable || loaded.LastHealthAt == nil {
		t.Fatalf("unexpected unavailable snapshot: %#v", loaded)
	}
	if loaded.LastHealthMessage != "connector unavailable" || strings.Contains(loaded.LastHealthMessage, "connection refused") {
		t.Fatalf("unexpected persisted failure summary: %q", loaded.LastHealthMessage)
	}
}

func TestServiceProbeBoundsSuccessfulHealthMessage(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	remote := &fakeRemoteClient{
		manifest: Manifest{ContractVersion: "1.0", Kind: "state_source_sync"},
		health:   Health{Status: "healthy", Message: strings.Repeat("x", 700)},
	}
	service := NewService(repository, remote)
	connector, err := service.Create(context.Background(), CreateConnectorRequest{
		Kind: "state_source_sync", DisplayName: "State Source", BaseURL: "http://sync:8989", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	if _, err := service.Probe(context.Background(), connector.ConnectorID); err != nil {
		t.Fatalf("probe connector: %v", err)
	}
	loaded, err := repository.Get(context.Background(), connector.ConnectorID)
	if err != nil {
		t.Fatalf("reload connector: %v", err)
	}
	if len([]rune(loaded.LastHealthMessage)) != 512 {
		t.Fatalf("health message length = %d, want 512", len([]rune(loaded.LastHealthMessage)))
	}
}

func TestServiceAllowsMultipleEnabledInstancesOfSameKind(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	service := NewService(repository, &fakeRemoteClient{})
	ctx := context.Background()
	first, err := service.Create(ctx, CreateConnectorRequest{
		Kind: "state_source_a", DisplayName: "Source A", BaseURL: "http://sync-a:8989", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := service.Create(ctx, CreateConnectorRequest{
		Kind: "state_source_a", DisplayName: "Source A 2", BaseURL: "http://sync-b:8989", Enabled: true,
	})
	if err != nil {
		t.Fatalf("second enabled instance of same kind must be allowed: %v", err)
	}
	third, err := service.Create(ctx, CreateConnectorRequest{
		Kind: "state_source_b", DisplayName: "Source B", BaseURL: "http://sync-c:8989", Enabled: true,
	})
	if err != nil {
		t.Fatalf("different kind must be allowed: %v", err)
	}
	for _, connectorID := range []string{first.ConnectorID, second.ConnectorID, third.ConnectorID} {
		if _, err := service.Get(ctx, connectorID); err != nil {
			t.Fatalf("get %s: %v", connectorID, err)
		}
	}
}

func TestServiceTriggerUsesStableGeneratedRequestID(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	remote := &fakeRemoteClient{triggeredRun: Run{RunID: "sync-run-1", Status: "running"}}
	service := NewService(repository, remote, WithRequestIDGenerator(func() string { return "request-fixed" }))
	connector, err := service.Create(context.Background(), CreateConnectorRequest{
		Kind: "state_source_sync", DisplayName: "State Source", BaseURL: "http://sync:8989", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}

	result, err := service.TriggerRun(context.Background(), connector.ConnectorID, "")
	if err != nil {
		t.Fatalf("trigger run: %v", err)
	}
	if result.RequestID != "request-fixed" || remote.triggerRequestID != "request-fixed" || result.Run.RunID != "sync-run-1" {
		t.Fatalf("unexpected trigger result: %#v remote=%q", result, remote.triggerRequestID)
	}
}

type fakeRemoteClient struct {
	manifest         Manifest
	manifestErr      error
	health           Health
	healthErr        error
	triggeredRun     Run
	triggerErr       error
	triggerRequestID string
	listRunsCalls    int
}

func (client *fakeRemoteClient) Manifest(ctx context.Context, baseURL string) (Manifest, error) {
	return client.manifest, client.manifestErr
}

func (client *fakeRemoteClient) Health(ctx context.Context, baseURL string) (Health, error) {
	return client.health, client.healthErr
}

func (client *fakeRemoteClient) TriggerRun(ctx context.Context, baseURL, requestID, repositoryID string) (Run, error) {
	client.triggerRequestID = requestID
	return client.triggeredRun, client.triggerErr
}

func (client *fakeRemoteClient) ListRuns(ctx context.Context, baseURL string, page PageRequest) (RemotePage[Run], error) {
	client.listRunsCalls++
	return RemotePage[Run]{}, nil
}

func (client *fakeRemoteClient) GetRun(ctx context.Context, baseURL, runID string) (Run, error) {
	return Run{}, nil
}

func (client *fakeRemoteClient) ListRecords(ctx context.Context, baseURL string, page PageRequest) (RemotePage[Record], error) {
	return RemotePage[Record]{}, nil
}

func (client *fakeRemoteClient) GetRecord(ctx context.Context, baseURL, recordID string) (Record, error) {
	return Record{}, nil
}

func (client *fakeRemoteClient) Metrics(ctx context.Context, baseURL, repositoryID string) (Metrics, error) {
	return Metrics{}, nil
}

func (client *fakeRemoteClient) Configuration(ctx context.Context, baseURL string) (Configuration, error) {
	return Configuration{Mode: "setup_pending"}, nil
}

func (client *fakeRemoteClient) SaveConfiguration(ctx context.Context, baseURL string, request SaveConfigurationRequest) (Configuration, error) {
	return Configuration{Mode: "ready", RestartRequired: true, Settings: request.Settings}, nil
}

func (client *fakeRemoteClient) PutRepository(ctx context.Context, baseURL, repositoryID string, request SaveRepositoryRequest) (RemoteRepository, error) {
	return RemoteRepository{RepositoryID: repositoryID, ConfigRevision: request.ConfigRevision, Enabled: request.Enabled}, nil
}

func (client *fakeRemoteClient) GetRepository(ctx context.Context, baseURL, repositoryID string) (RemoteRepository, error) {
	return RemoteRepository{RepositoryID: repositoryID}, nil
}

func (client *fakeRemoteClient) DeleteRepository(ctx context.Context, baseURL, repositoryID string) error {
	return nil
}
