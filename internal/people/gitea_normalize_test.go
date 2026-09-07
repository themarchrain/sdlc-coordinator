package people

import "testing"

func TestNormalizeGiteaLookupRequest(t *testing.T) {
	baseURL, username, err := normalizeGiteaLookupRequest(LookupRequest{
		Provider: "gitea", BaseURL: "https://gitea.example.com/", Token: "token", Email: "user@example.com",
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if baseURL != "https://gitea.example.com" || username != "user" {
		t.Fatalf("unexpected normalize result: %q %q", baseURL, username)
	}

	if _, _, err := normalizeGiteaLookupRequest(LookupRequest{
		Provider: "gitlab", BaseURL: "https://gitea.example.com", Token: "token", Email: "user@example.com",
	}); err == nil {
		t.Fatal("expected gitlab provider to be rejected by gitea normalize")
	}
}
