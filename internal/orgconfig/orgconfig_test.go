package orgconfig

import (
	"context"
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "orgconfig.db")) + "?_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := NewStore(db).Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestStoreLoadWithoutRowReturnsNotConfigured(t *testing.T) {
	db := newTestDB(t)

	config, revision, configured, err := NewStore(db).Load(context.Background())
	if err != nil || configured || revision != 0 {
		t.Fatalf("unexpected empty-load result: config=%+v rev=%d configured=%v err=%v", config, revision, configured, err)
	}
}

func TestStoreSaveRoundTripAndRevisionIncrement(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	store := NewStore(db)

	first := configuration.CodeHostConfig{Provider: "github", BaseURL: "https://github.com", ReworkAllowedUserIDs: []string{"101"}}
	revision1, err := store.Save(ctx, Config{CodeHost: first})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	if revision1 != 1 {
		t.Fatalf("first revision = %d, want 1", revision1)
	}

	loaded, revision, configured, err := store.Load(ctx)
	if err != nil || !configured || revision != 1 {
		t.Fatalf("unexpected load result: %+v rev=%d configured=%v err=%v", loaded, revision, configured, err)
	}
	if loaded.CodeHost.Provider != "github" || len(loaded.CodeHost.ReworkAllowedUserIDs) != 1 || loaded.CodeHost.ReworkAllowedUserIDs[0] != "101" {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}

	second := Config{CodeHost: first, IM: configuration.IMConfig{Provider: "feishu", Feishu: configuration.IMFeishuConfig{WebhookURL: "https://example.com/hook"}}}
	revision2, err := store.Save(ctx, second)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if revision2 != 2 {
		t.Fatalf("second revision = %d, want 2", revision2)
	}
	loaded, _, _, err = store.Load(ctx)
	if err != nil || loaded.IM.Provider != "feishu" {
		t.Fatalf("second round trip mismatch: %+v err=%v", loaded, err)
	}
}
