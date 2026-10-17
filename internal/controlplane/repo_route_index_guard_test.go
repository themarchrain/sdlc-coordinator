package controlplane

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/store/storetest"
)

func TestRepoRouteIndexGuardsSnapshotWrites(t *testing.T) {
	for _, backend := range []string{"sqlite", "mysql"} {
		t.Run(backend, func(t *testing.T) {
			for _, action := range []string{"delete", "replace", "insert", "recreated", "current_replace", "current_delete"} {
				t.Run(action, func(t *testing.T) {
					repo := openTestControlPlane(t)
					if backend == "mysql" {
						repo = NewRepository(storetest.OpenDatabase(t, storetest.RequireMySQL(t), "route_guard"))
					}
					if err := repo.Migrate(); err != nil {
						t.Fatal(err)
					}
					ctx := context.Background()
					identity := RepoRouteIdentity{Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "group/repo"}
					row := RepoRouteIndex{
						Provider: identity.Provider, BaseURL: identity.BaseURL, PathWithNamespace: identity.PathWithNamespace,
						TenantID: 7, RepoFingerprint: "repo",
					}
					if err := repo.UpsertRepoRouteIndex(ctx, row); err != nil {
						t.Fatal(err)
					}
					before, err := repo.GetRepoRouteIndex(ctx, identity)
					if err != nil {
						t.Fatal(err)
					}
					if action == "recreated" {
						if err := repo.DeleteRepoRouteIndex(ctx, identity); err != nil {
							t.Fatal(err)
						}
					}
					if err := repo.UpsertRepoRouteIndex(ctx, row); err != nil {
						t.Fatal(err)
					}
					written, err := repo.GetRepoRouteIndex(ctx, identity)
					if err != nil {
						t.Fatal(err)
					}
					if action != "recreated" && written.Version <= before.Version {
						t.Fatal("a write-through with unchanged attributes did not advance the route version")
					}
					if action == "recreated" && written.ID == before.ID {
						t.Fatal("the replacement route reused its deleted row identity")
					}
					var applied bool
					switch action {
					case "delete", "recreated":
						applied, err = repo.DeleteRepoRouteIndexGuarded(ctx, *before)
					case "replace":
						row.TenantID, row.RepoFingerprint = 9, "stale-repair"
						applied, err = repo.UpsertRepoRouteIndexGuarded(ctx, row, before)
					case "insert":
						row.TenantID, row.RepoFingerprint = 9, "stale-insert"
						applied, err = repo.UpsertRepoRouteIndexGuarded(ctx, row, nil)
					case "current_replace":
						row.TenantID, row.RepoFingerprint = 9, "valid-repair"
						applied, err = repo.UpsertRepoRouteIndexGuarded(ctx, row, written)
					case "current_delete":
						applied, err = repo.DeleteRepoRouteIndexGuarded(ctx, *written)
					}
					if err != nil {
						t.Fatal(err)
					}
					wantApplied := action == "current_replace" || action == "current_delete"
					if applied != wantApplied {
						t.Fatalf("guarded write applied=%t, want %t", applied, wantApplied)
					}
					current, err := repo.GetRepoRouteIndex(ctx, identity)
					if action == "current_delete" {
						if !errors.Is(err, ErrRepoRouteNotFound) {
							t.Fatalf("current route was not deleted: route=%+v err=%v", current, err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if action == "current_replace" {
						if current.TenantID != 9 || current.RepoFingerprint != "valid-repair" || current.Version <= written.Version {
							t.Fatalf("valid repair was not persisted: %+v", current)
						}
					} else if current.TenantID != 7 || current.RepoFingerprint != "repo" || current.Version != written.Version {
						t.Fatalf("stale snapshot modified the current route: %+v", current)
					}
				})
			}
		})
	}
}
