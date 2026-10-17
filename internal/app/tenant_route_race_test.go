package app

import (
	"context"
	"sync"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/tenantdb"

	"gorm.io/gorm"
)

func TestReconcileRepoRouteIndexPreservesConcurrentIdentityChange(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new_route"
		if existing {
			name = "updated_route"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			cp := openControlPlaneRepository(t, testControlPlaneSettings(t))
			tenant := &controlplane.Tenant{Slug: "route-race", DisplayName: "Route race"}
			if err := cp.CreateTenant(ctx, tenant); err != nil {
				t.Fatal(err)
			}
			resolver := tenantdb.NewSqliteFileResolver(nil, t.TempDir(), 0)
			t.Cleanup(resolver.Close)
			handle, err := resolver.Handle(ctx, tenant.ID)
			if err != nil {
				t.Fatal(err)
			}
			repos := registeredrepo.NewStore(handle.DB)
			if err := repos.Migrate(); err != nil {
				t.Fatal(err)
			}
			if err := repos.Create(ctx, &registeredrepo.RegisteredRepository{
				RepositoryID: "repo", DisplayName: "Repository", Provider: "gitlab",
				BaseURL: "https://gitlab.example.com", PathWithNamespace: "group/old",
				WebhookSecret: "secret", Status: registeredrepo.StatusEnabled,
			}); err != nil {
				t.Fatal(err)
			}
			route := controlplane.RepoRouteIndex{
				Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "group/old",
				TenantID: tenant.ID, RepoFingerprint: "repo",
			}
			if err := cp.UpsertRepoRouteIndex(ctx, route); err != nil {
				t.Fatal(err)
			}
			route.PathWithNamespace = "group/new"
			if existing {
				if err := cp.UpsertRepoRouteIndex(ctx, route); err != nil {
					t.Fatal(err)
				}
			}
			var once sync.Once
			err = handle.DB.Callback().Query().After("gorm:query").Register("test:change-repository-after-scan", func(tx *gorm.DB) {
				if tx.Statement.Table != "registered_repositories" {
					return
				}
				once.Do(func() {
					if err := handle.DB.WithContext(ctx).Model(&registeredrepo.RegisteredRepository{}).
						Where("repository_id = ?", "repo").Update("path_with_namespace", "group/new").Error; err != nil {
						t.Fatalf("update authoritative repository: %v", err)
					}
					if err := cp.UpsertRepoRouteIndex(ctx, route); err != nil {
						t.Fatalf("write through repository route: %v", err)
					}
				})
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := reconcileRepoRouteIndex(ctx, cp, resolver, nil); err != nil {
				t.Fatal(err)
			}
			current, err := repos.GetByPath(ctx, "gitlab", "group/new")
			if err != nil || current.Status != registeredrepo.StatusEnabled {
				t.Fatalf("changed repository is not active: repo=%+v err=%v", current, err)
			}
			secret, err := (&tenantRouteSecretLookup{index: cp, resolver: resolver}).WebhookSecretByPath(ctx, "gitlab", "group/new", "")
			if err != nil || secret != "secret" {
				t.Fatalf("reconciliation lost the current webhook route: secret=%q err=%v", secret, err)
			}
		})
	}
}
