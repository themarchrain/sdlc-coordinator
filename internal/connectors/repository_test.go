package connectors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	storerepository "sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

func TestConnectorRepositoryCRUDDefaultsAndUniqueness(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	ctx := context.Background()
	connector := &model.ConnectorInstance{
		Kind:              "state_source_sync",
		DisplayName:       "State Source Sync",
		BaseURL:           "http://sync:8989",
		Enabled:           true,
		TargetProjectPath: "delper-ai/delper-agent",
	}

	if err := repository.Create(ctx, connector); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	if connector.ConnectorID == "" {
		t.Fatal("expected generated connector id")
	}
	if connector.LastHealthStatus != model.ConnectorHealthUnknown {
		t.Fatalf("expected unknown health, got %q", connector.LastHealthStatus)
	}

	loaded, err := repository.Get(ctx, connector.ConnectorID)
	if err != nil {
		t.Fatalf("get connector: %v", err)
	}
	if loaded.BaseURL != connector.BaseURL || loaded.DisplayName != connector.DisplayName {
		t.Fatalf("unexpected connector: %#v", loaded)
	}

	loaded.DisplayName = "Renamed Connector"
	loaded.Enabled = false
	if err := repository.Update(ctx, loaded); err != nil {
		t.Fatalf("update connector: %v", err)
	}
	updated, err := repository.Get(ctx, connector.ConnectorID)
	if err != nil || updated.DisplayName != "Renamed Connector" || updated.Enabled {
		t.Fatalf("unexpected updated connector: %#v %v", updated, err)
	}

	duplicate := &model.ConnectorInstance{Kind: connector.Kind, DisplayName: "Duplicate", BaseURL: connector.BaseURL}
	if err := repository.Create(ctx, duplicate); err == nil {
		t.Fatal("expected duplicate kind and base_url to be rejected")
	}

	if err := repository.Delete(ctx, connector.ConnectorID); err != nil {
		t.Fatalf("delete connector: %v", err)
	}
	if _, err := repository.Get(ctx, connector.ConnectorID); !errors.Is(err, storerepository.ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestConnectorRepositoryListFiltersAndPaginates(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	ctx := context.Background()
	connectors := []*model.ConnectorInstance{
		{Kind: "state_source_sync", DisplayName: "A", BaseURL: "http://sync-a:8989", Enabled: true, TargetProjectPath: "group/repo-a"},
		{Kind: "state_source_sync", DisplayName: "B", BaseURL: "http://sync-b:8989", Enabled: false, TargetProjectPath: "group/repo-b"},
		{Kind: "sre_alert_sync", DisplayName: "C", BaseURL: "http://sync-c:8989", Enabled: true, TargetProjectPath: "group/repo-a"},
	}
	for _, connector := range connectors {
		if err := repository.Create(ctx, connector); err != nil {
			t.Fatalf("create connector %s: %v", connector.DisplayName, err)
		}
	}

	enabled := true
	page, err := repository.List(ctx, ConnectorFilter{
		PageRequest:       PageRequest{Page: 0, PageSize: 500},
		Enabled:           &enabled,
		TargetProjectPath: "group/repo-a",
	})
	if err != nil {
		t.Fatalf("list connectors: %v", err)
	}
	if page.Page != 1 || page.PageSize != 100 || page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("unexpected page: %#v", page)
	}

	kindPage, err := repository.List(ctx, ConnectorFilter{Kind: "state_source_sync"})
	if err != nil || kindPage.Total != 2 {
		t.Fatalf("unexpected kind page: %#v %v", kindPage, err)
	}
}

func TestConnectorRepositoryHealthSnapshotCannotOverwriteChangedConfiguration(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	ctx := context.Background()
	connector := &model.ConnectorInstance{Kind: "state_source_sync", DisplayName: "State Source", BaseURL: "http://sync-old:8989", Enabled: true}
	if err := repository.Create(ctx, connector); err != nil {
		t.Fatalf("create connector: %v", err)
	}

	loaded, err := repository.Get(ctx, connector.ConnectorID)
	if err != nil {
		t.Fatalf("load connector: %v", err)
	}
	loaded.BaseURL = "http://sync-new:8989"
	if err := repository.Update(ctx, loaded); err != nil {
		t.Fatalf("update connector url: %v", err)
	}

	capabilities := `["manual_trigger"]`
	err = repository.UpdateHealthSnapshot(ctx, connector.ConnectorID, "http://sync-old:8989", HealthSnapshot{
		Status: model.ConnectorHealthHealthy, Message: "stale", At: time.Now().UTC(), CapabilitiesJSON: &capabilities,
	})
	if !errors.Is(err, ErrConnectorChanged) {
		t.Fatalf("expected connector changed, got %v", err)
	}
	current, err := repository.Get(ctx, connector.ConnectorID)
	if err != nil {
		t.Fatalf("reload connector: %v", err)
	}
	if current.BaseURL != "http://sync-new:8989" || current.LastHealthStatus != model.ConnectorHealthUnknown || current.CapabilitiesJSON != "[]" {
		t.Fatalf("stale health update overwrote configuration: %#v", current)
	}
}

func TestConnectorRepositoryHealthSnapshotCannotReviveDeletedConnector(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	ctx := context.Background()
	connector := &model.ConnectorInstance{Kind: "state_source_sync", DisplayName: "State Source", BaseURL: "http://sync:8989", Enabled: true}
	if err := repository.Create(ctx, connector); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	if err := repository.Delete(ctx, connector.ConnectorID); err != nil {
		t.Fatalf("delete connector: %v", err)
	}

	err := repository.UpdateHealthSnapshot(ctx, connector.ConnectorID, connector.BaseURL, HealthSnapshot{
		Status: model.ConnectorHealthUnavailable, Message: "late response", At: time.Now().UTC(),
	})
	if !errors.Is(err, storerepository.ErrNotFound) {
		t.Fatalf("expected not found after concurrent delete, got %v", err)
	}
	if _, err := repository.Get(ctx, connector.ConnectorID); !errors.Is(err, storerepository.ErrNotFound) {
		t.Fatalf("deleted connector was revived: %v", err)
	}
}

func TestConnectorRepositoryConfigurationUpdatePreservesConcurrentHealthAndCannotReviveDelete(t *testing.T) {
	repository, _ := newConnectorRepositoryTestDB(t)
	ctx := context.Background()
	connector := &model.ConnectorInstance{Kind: "state_source_sync", DisplayName: "State Source", BaseURL: "http://sync:8989", Enabled: true}
	if err := repository.Create(ctx, connector); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	staleConfiguration, err := repository.Get(ctx, connector.ConnectorID)
	if err != nil {
		t.Fatalf("load connector: %v", err)
	}
	if err := repository.UpdateHealthSnapshot(ctx, connector.ConnectorID, connector.BaseURL, HealthSnapshot{
		Status: model.ConnectorHealthHealthy, Message: "ready", At: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("update health: %v", err)
	}
	staleConfiguration.DisplayName = "Renamed"
	if err := repository.Update(ctx, staleConfiguration); err != nil {
		t.Fatalf("update configuration: %v", err)
	}
	current, err := repository.Get(ctx, connector.ConnectorID)
	if err != nil {
		t.Fatalf("reload connector: %v", err)
	}
	if current.DisplayName != "Renamed" || current.LastHealthStatus != model.ConnectorHealthHealthy || current.LastHealthMessage != "ready" {
		t.Fatalf("configuration update overwrote health snapshot: %#v", current)
	}

	if err := repository.Delete(ctx, connector.ConnectorID); err != nil {
		t.Fatalf("delete connector: %v", err)
	}
	if err := repository.Update(ctx, staleConfiguration); !errors.Is(err, storerepository.ErrNotFound) {
		t.Fatalf("expected not found for stale update after delete, got %v", err)
	}
	if _, err := repository.Get(ctx, connector.ConnectorID); !errors.Is(err, storerepository.ErrNotFound) {
		t.Fatalf("configuration update revived deleted connector: %v", err)
	}
}

func newConnectorRepositoryTestDB(t *testing.T) (*ConnectorRepository, *gorm.DB) {
	t.Helper()
	dsnName := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	db, err := store.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dsnName))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.ConnectorInstance{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return NewConnectorRepository(db), db
}
