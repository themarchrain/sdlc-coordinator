package controlplane

import (
	"context"
	"testing"
)

// 控制面持只读路由索引 (provider, base_url, path) → tenant，
// webhook 投递据此路由租户库；写穿与对账是唯一写入方。
func TestRepoRouteIndexCRUD(t *testing.T) {
	repository := openTestControlPlane(t)
	ctx := context.Background()
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}

	identity := RepoRouteIdentity{Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/app"}
	if err := repository.UpsertRepoRouteIndex(ctx, RepoRouteIndex{
		Provider: identity.Provider, BaseURL: identity.BaseURL, PathWithNamespace: identity.PathWithNamespace,
		TenantID: 7, RepoFingerprint: "repo-1",
	}); err != nil {
		t.Fatalf("upsert index: %v", err)
	}

	route, err := repository.GetRepoRouteIndex(ctx, identity)
	if err != nil || route.TenantID != 7 || route.RepoFingerprint != "repo-1" {
		t.Fatalf("get route: route=%#v err=%v", route, err)
	}

	// 同身份再写 = 更新（改租户/指纹），不产生重复行。
	if err := repository.UpsertRepoRouteIndex(ctx, RepoRouteIndex{
		Provider: identity.Provider, BaseURL: identity.BaseURL, PathWithNamespace: identity.PathWithNamespace,
		TenantID: 9, RepoFingerprint: "repo-1b",
	}); err != nil {
		t.Fatalf("re-upsert index: %v", err)
	}
	route, err = repository.GetRepoRouteIndex(ctx, identity)
	if err != nil || route.TenantID != 9 {
		t.Fatalf("re-upsert must overwrite: route=%#v err=%v", route, err)
	}

	rows, err := repository.ListRepoRouteIndex(ctx, 9)
	if err != nil || len(rows) != 1 || rows[0].PathWithNamespace != "acme/app" {
		t.Fatalf("list by tenant: rows=%#v err=%v", rows, err)
	}

	if err := repository.DeleteRepoRouteIndex(ctx, identity); err != nil {
		t.Fatalf("delete index: %v", err)
	}
	if _, err := repository.GetRepoRouteIndex(ctx, identity); err == nil {
		t.Fatalf("deleted route must not resolve")
	}
}
