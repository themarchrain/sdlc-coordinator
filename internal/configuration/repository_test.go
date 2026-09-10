package configuration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/store"
)

func TestRepositoryReturnsNotFoundWhenNoSnapshotExists(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)

	_, err := repository.Latest(context.Background())

	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected snapshot not found, got %v", err)
	}
}

func TestRepositoryMarksSavedRevisionAppliedWithoutChangingPayload(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	snapshot, err := repository.Save(context.Background(), StatusRestartRequired, SourceUI, validRuntimeConfig())
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	appliedAt := time.Date(2026, 8, 10, 10, 30, 0, 0, time.UTC)

	if err := repository.MarkApplied(context.Background(), snapshot.Revision, appliedAt); err != nil {
		t.Fatalf("mark applied: %v", err)
	}
	latest, err := repository.Latest(context.Background())
	if err != nil {
		t.Fatalf("load applied snapshot: %v", err)
	}
	if latest.Status != StatusReady || latest.AppliedAt == nil || !latest.AppliedAt.Equal(appliedAt) {
		t.Fatalf("unexpected applied metadata: %#v", latest)
	}
	if latest.Config.AgentRunner.Model != validRuntimeConfig().AgentRunner.Model {
		t.Fatal("marking applied changed payload")
	}
}

func TestRepositorySavesIncreasingCompleteSnapshots(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	ctx := context.Background()
	firstConfig := validRuntimeConfig()
	secondConfig := validRuntimeConfig()
	secondConfig.AgentRunner.Model = "model-two"

	first, err := repository.Save(ctx, StatusReady, SourceEnvImport, firstConfig)
	if err != nil {
		t.Fatalf("save first snapshot: %v", err)
	}
	second, err := repository.Save(ctx, StatusRestartRequired, SourceUI, secondConfig)
	if err != nil {
		t.Fatalf("save second snapshot: %v", err)
	}

	if first.Revision != 1 || second.Revision != 2 {
		t.Fatalf("unexpected revisions: %d %d", first.Revision, second.Revision)
	}
	latest, err := repository.Latest(ctx)
	if err != nil {
		t.Fatalf("load latest snapshot: %v", err)
	}
	if latest.Revision != 2 || latest.Status != StatusRestartRequired || latest.Source != SourceUI {
		t.Fatalf("unexpected latest metadata: %#v", latest)
	}
	if latest.Config.AgentRunner.Model != secondConfig.AgentRunner.Model || latest.Config.AgentRunner.APIKey != secondConfig.AgentRunner.APIKey {
		t.Fatalf("latest snapshot is incomplete: %#v", latest.Config)
	}

	var rows int64
	if err := repository.db.Model(&SnapshotRecord{}).Count(&rows).Error; err != nil {
		t.Fatalf("count snapshot rows: %v", err)
	}
	if rows != 2 {
		t.Fatalf("expected immutable history rows, got %d", rows)
	}
}

func TestRepositoryReportsMalformedLatestPayload(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	if err := repository.db.Create(&SnapshotRecord{
		Revision:    1,
		Status:      string(StatusReady),
		Source:      string(SourceUI),
		PayloadJSON: "{not-json",
	}).Error; err != nil {
		t.Fatalf("seed malformed row: %v", err)
	}

	_, err := repository.Latest(context.Background())

	if err == nil || !strings.Contains(err.Error(), "decode runtime configuration snapshot") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func newConfigurationRepositoryTestDB(t *testing.T) *Repository {
	t.Helper()
	dsnName := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	db, err := store.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dsnName))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repository := NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate repository: %v", err)
	}
	return repository
}
