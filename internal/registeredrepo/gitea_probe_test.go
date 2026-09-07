package registeredrepo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGiteaHTTPProbeReturnsProjectIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/group/repo" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "token repo-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"full_name":"group/repo","default_branch":"main","clone_url":"https://gitea.example/group/repo.git"}`))
	}))
	defer server.Close()

	probe := NewGiteaHTTPProbe(nil)
	ident, err := probe.GetProject(context.Background(), server.URL, "repo-token", "group/repo")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if ident.ID != "42" || ident.PathWithNamespace != "group/repo" || ident.DefaultBranch != "main" {
		t.Fatalf("unexpected identity: %#v", ident)
	}
}

func TestGiteaHTTPProbeRejectsMissingProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := NewGiteaHTTPProbe(nil).GetProject(context.Background(), server.URL, "t", "group/missing"); err == nil {
		t.Fatal("expected error for missing project")
	}
}

// 仓库注册链路的身份探针必须按 provider 分流,否则 Gitea 仓库会被拿去问
// GitLab API,注册后永远停在 configuring。
func TestNewProviderProbeSelectsProviderProbe(t *testing.T) {
	if _, ok := NewProviderProbe("gitea", nil).(GiteaHTTPProbe); !ok {
		t.Fatal("expected gitea provider to select GiteaHTTPProbe")
	}
	if _, ok := NewProviderProbe("gitlab", nil).(HTTPProbe); !ok {
		t.Fatal("expected gitlab provider to select HTTPProbe")
	}
	if _, ok := NewProviderProbe("", nil).(HTTPProbe); !ok {
		t.Fatal("expected empty provider to default to HTTPProbe")
	}
}
