package registeredrepo

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func createGitIdentityTestRepository(t *testing.T, svc *Service, name, email string) *View {
	t.Helper()
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:        "A",
		PathWithNamespace:  "group/a",
		Provider:           "gitlab",
		BaseURL:            "https://gitlab.example.com",
		Token:              "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
		GitAuthorName:      name,
		GitAuthorEmail:     email,
	})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func forceRepositoryStatus(t *testing.T, svc *Service, repositoryID string, status RepositoryStatus) {
	t.Helper()
	stored, err := svc.store.Get(context.Background(), repositoryID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Status = status
	if err := svc.store.Update(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
}

func ptrString(value string) *string { return &value }

func TestCreateRepositoryAcceptsPairedGitIdentity(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view := createGitIdentityTestRepository(t, svc, "Release Bot", "bot@example.com")
	if view.GitAuthorName != "Release Bot" || view.GitAuthorEmail != "bot@example.com" {
		t.Fatalf("git identity not echoed: %+v", view)
	}
}

func TestCreateRepositoryKeepsIdentityOptional(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view := createGitIdentityTestRepository(t, svc, "", "")
	if view.GitAuthorName != "" || view.GitAuthorEmail != "" {
		t.Fatalf("empty identity must stay empty: %+v", view)
	}
}

func TestCreateRepositoryRejectsUnpairedGitIdentity(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	if _, err := svc.Create(context.Background(), CreateRequest{
		DisplayName: "A", PathWithNamespace: "group/a", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", Token: "token-a", TokenScopeAttested: true,
		WebhookSecret: "hook-secret-a", GitAuthorName: "Only Name",
	}); !errors.Is(err, ErrInvalidGitIdentity) {
		t.Fatalf("name without email must be rejected: %v", err)
	}
	if _, err := svc.Create(context.Background(), CreateRequest{
		DisplayName: "A", PathWithNamespace: "group/a", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", Token: "token-a", TokenScopeAttested: true,
		WebhookSecret: "hook-secret-a", GitAuthorEmail: "only@example.com",
	}); !errors.Is(err, ErrInvalidGitIdentity) {
		t.Fatalf("email without name must be rejected: %v", err)
	}
}

func TestCreateRepositoryRejectsInvalidGitIdentityValues(t *testing.T) {
	cases := []struct {
		name  string
		email string
	}{
		{"line\nbreak", "bot@example.com"},
		{"cr\rbreak", "bot@example.com"},
		{strings.Repeat("长", 129), "bot@example.com"},
		{"Release Bot", "not-an-email"},
		{"Release Bot", "two@@example.com"},
		{"Release Bot", "bot@example.com " + strings.Repeat("长", 240)},
	}
	for _, tc := range cases {
		svc, _ := newServiceWithProbe(t)
		if _, err := svc.Create(context.Background(), CreateRequest{
			DisplayName: "A", PathWithNamespace: "group/a", Provider: "gitlab",
			BaseURL: "https://gitlab.example.com", Token: "token-a", TokenScopeAttested: true,
			WebhookSecret: "hook-secret-a", GitAuthorName: tc.name, GitAuthorEmail: tc.email,
		}); !errors.Is(err, ErrInvalidGitIdentity) {
			t.Fatalf("identity %q/%q must be rejected: %v", tc.name, tc.email, err)
		}
	}
}

func TestPatchGitIdentityOnEnabledRequiresDisable(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view := createGitIdentityTestRepository(t, svc, "Release Bot", "bot@example.com")
	forceRepositoryStatus(t, svc, view.RepositoryID, StatusEnabled)

	newName := "Other Bot"
	newEmail := "other@example.com"
	if _, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{
		GitAuthorName: &newName, GitAuthorEmail: &newEmail,
	}); !errors.Is(err, ErrGitIdentityRequiresDisabled) {
		t.Fatalf("identity change on enabled repository must require disable first: %v", err)
	}

	// 表单原样回传当前值不算变更：enabled 仓改其他字段时不得被身份门误伤。
	currentName := "Release Bot"
	currentEmail := "bot@example.com"
	if _, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{
		GitAuthorName: &currentName, GitAuthorEmail: &currentEmail, VerifyCommand: ptrString("go test ./..."),
	}); err != nil {
		t.Fatalf("unchanged identity values must pass the gate: %v", err)
	}
}

func TestPatchGitIdentityOnDisabledSetsUpdatesAndClears(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view := createGitIdentityTestRepository(t, svc, "", "")
	forceRepositoryStatus(t, svc, view.RepositoryID, StatusDisabled)

	newName := "Release Bot"
	newEmail := "bot@example.com"
	patched, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{
		GitAuthorName: &newName, GitAuthorEmail: &newEmail,
	})
	if err != nil {
		t.Fatalf("set identity on disabled repository: %v", err)
	}
	if patched.GitAuthorName != "Release Bot" || patched.GitAuthorEmail != "bot@example.com" {
		t.Fatalf("identity not stored: %+v", patched)
	}

	updatedName := "Renamed Bot"
	updatedEmail := "renamed@example.com"
	if _, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{
		GitAuthorName: &updatedName, GitAuthorEmail: &updatedEmail,
	}); err != nil {
		t.Fatalf("update identity: %v", err)
	}

	clearName := ""
	clearEmail := ""
	cleared, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{
		GitAuthorName: &clearName, GitAuthorEmail: &clearEmail,
	})
	if err != nil {
		t.Fatalf("clear identity on disabled repository: %v", err)
	}
	if cleared.GitAuthorName != "" || cleared.GitAuthorEmail != "" {
		t.Fatalf("identity not cleared: %+v", cleared)
	}
}

func TestPatchGitIdentityHalfPairRejected(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view := createGitIdentityTestRepository(t, svc, "", "")
	forceRepositoryStatus(t, svc, view.RepositoryID, StatusDisabled)

	newName := "Only Name"
	if _, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{GitAuthorName: &newName}); !errors.Is(err, ErrInvalidGitIdentity) {
		t.Fatalf("name-only patch must be rejected: %v", err)
	}
	newEmail := "only@example.com"
	if _, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{GitAuthorEmail: &newEmail}); !errors.Is(err, ErrInvalidGitIdentity) {
		t.Fatalf("email-only patch must be rejected: %v", err)
	}
}

func TestPatchGitIdentityKeepsDisabledStatus(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view := createGitIdentityTestRepository(t, svc, "", "")
	forceRepositoryStatus(t, svc, view.RepositoryID, StatusDisabled)

	newName := "Release Bot"
	newEmail := "bot@example.com"
	patched, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{
		GitAuthorName: &newName, GitAuthorEmail: &newEmail,
	})
	if err != nil {
		t.Fatalf("set identity on disabled repository: %v", err)
	}
	if patched.Status != StatusDisabled {
		t.Fatalf("identity change must not reconfigure a disabled repository, status=%s", patched.Status)
	}
}

func TestPatchCosmeticFieldsPreserveEnabledStatus(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view := createGitIdentityTestRepository(t, svc, "", "")
	forceRepositoryStatus(t, svc, view.RepositoryID, StatusEnabled)

	patched, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{
		VerifyCommand: ptrString("go test ./..."),
	})
	if err != nil {
		t.Fatalf("cosmetic patch on enabled repository: %v", err)
	}
	if patched.Status != StatusEnabled {
		t.Fatalf("cosmetic fields must not reconfigure an enabled repository, status=%s", patched.Status)
	}
}

func TestLegacyMigrationLeavesGitIdentityEmpty(t *testing.T) {
	repoStore := openLegacySchemaTestDB(t)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	row, err := repoStore.Get(context.Background(), "repo_legacy_a")
	if err != nil {
		t.Fatal(err)
	}
	if row.GitAuthorName != "" || row.GitAuthorEmail != "" {
		t.Fatalf("legacy rows must migrate with empty git identity: %+v", row)
	}
	if !hasColumn(t, repoStore, "git_author_name") || !hasColumn(t, repoStore, "git_author_email") {
		t.Fatal("git identity columns missing after migration")
	}
}
