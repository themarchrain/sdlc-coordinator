package orgconfig

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/configuration"
)

func TestProviderReturnsFreshConfigAfterSave(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	provider := NewProvider(db)

	config, err := provider.Get(ctx)
	if err != nil || config.CodeHost.Provider != "" {
		t.Fatalf("empty provider result: %+v err=%v", config, err)
	}

	_, err = NewStore(db).Save(ctx, Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitea", BaseURL: "https://gitea.example.com", ReworkAllowedUserIDs: []string{"5"}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	config, err = provider.Get(ctx)
	if err != nil || config.CodeHost.Provider != "gitea" {
		t.Fatalf("provider did not observe save: %+v err=%v", config, err)
	}
	if ids := config.ReworkAllowedIDs(); len(ids) != 1 || ids[0] != 5 {
		t.Fatalf("rework ids mismatch: %v", ids)
	}
}

func TestProviderReuseAcrossRevision(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	provider := NewProvider(db)

	if _, err := provider.Get(ctx); err != nil {
		t.Fatalf("first get: %v", err)
	}
	if _, err := provider.Get(ctx); err != nil {
		t.Fatalf("second get: %v", err)
	}
}
