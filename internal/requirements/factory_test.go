package requirements

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/adapters/gitea"
	"sdlc-coordinator/internal/adapters/github"
	"sdlc-coordinator/internal/adapters/gitlab"
	"sdlc-coordinator/internal/registeredrepo"
)

// TestProviderClientFactoryDispatchesPerRepository 锁定按仓分派:client 类型由
// 仓行 provider 决定;平台身份缺失时返回恒错 client(fail-closed 落账不 panic)。
func TestProviderClientFactoryDispatchesPerRepository(t *testing.T) {
	factory := NewProviderClientFactory(nil)
	cases := map[string]func(any) bool{
		"gitea":  func(value any) bool { _, ok := value.(*gitea.Client); return ok },
		"github": func(value any) bool { _, ok := value.(*github.Client); return ok },
		"gitlab": func(value any) bool { _, ok := value.(*gitlab.Client); return ok },
	}
	for provider, check := range cases {
		repo := &registeredrepo.RegisteredRepository{
			Provider: provider, BaseURL: "http://codehost.local/", Token: "token-1",
			PathWithNamespace: "group/demo", CodeHostProjectID: "42",
		}
		client := factory(repo)
		if !check(client) {
			t.Fatalf("provider %q built wrong client type %T", provider, client)
		}
	}

	broken := &registeredrepo.RegisteredRepository{Provider: "gitbucket", BaseURL: "http://codehost.local/", Token: "t"}
	client := factory(broken)
	if _, err := client.CreateIssue(context.Background(), "group/demo", codehost.CreateIssueRequest{Title: "x"}); err == nil {
		t.Fatal("missing platform identity must yield failing client")
	}
}

func TestResolveProjectIDPrefersCodeHostProjectID(t *testing.T) {
	gitlabRepo := &registeredrepo.RegisteredRepository{Provider: "gitlab", CodeHostProjectID: "42", PathWithNamespace: "group/demo"}
	if got := ResolveProjectID(gitlabRepo); got != "42" {
		t.Fatalf("gitlab must prefer numeric project id, got %q", got)
	}
	giteaRepo := &registeredrepo.RegisteredRepository{Provider: "gitea", CodeHostProjectID: "42", PathWithNamespace: "group/demo"}
	if got := ResolveProjectID(giteaRepo); got != "group/demo" {
		t.Fatalf("gitea must use path, got %q", got)
	}
	noID := &registeredrepo.RegisteredRepository{Provider: "gitlab", PathWithNamespace: "group/demo"}
	if got := ResolveProjectID(noID); got != "group/demo" {
		t.Fatalf("gitlab falls back to path, got %q", got)
	}
}
