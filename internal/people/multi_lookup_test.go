package people

import (
	"context"
	"testing"
)

type recordingLookup struct {
	provider string
	requests int
}

func (r *recordingLookup) Lookup(_ context.Context, request LookupRequest) (LookupResult, error) {
	r.provider = request.Provider
	r.requests++
	return LookupResult{}, nil
}

// TestMultiProviderLookupDispatchesByRequestProvider 锁定分派:同一 lookup 按
// 请求自带的 provider 路由到对应平台 client,未知 provider 拒绝。
func TestMultiProviderLookupDispatchesByRequestProvider(t *testing.T) {
	gitlab, gitea, github := &recordingLookup{}, &recordingLookup{}, &recordingLookup{}
	lookup := newMultiProviderLookup(gitlab, gitea, github)
	ctx := context.Background()

	if _, err := lookup.Lookup(ctx, LookupRequest{Provider: "gitea", BaseURL: "https://gitea.example.com", Token: "t", Email: "a@b.c"}); err != nil {
		t.Fatalf("gitea lookup: %v", err)
	}
	if gitea.requests != 1 || gitlab.requests != 0 || github.requests != 0 {
		t.Fatalf("gitea dispatch: %+v", map[string]int{"gitlab": gitlab.requests, "gitea": gitea.requests, "github": github.requests})
	}
	if _, err := lookup.Lookup(ctx, LookupRequest{Provider: "GITHUB", BaseURL: "https://github.example.com", Token: "t", Email: "a@b.c"}); err != nil {
		t.Fatalf("github lookup: %v", err)
	}
	if github.requests != 1 {
		t.Fatal("provider matching must be case-insensitive")
	}
	if _, err := lookup.Lookup(ctx, LookupRequest{Provider: "gitbucket", BaseURL: "https://x.example.com", Token: "t", Email: "a@b.c"}); err == nil {
		t.Fatal("unknown provider must be rejected")
	}
}
