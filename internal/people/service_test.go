package people_test

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/people"
	"sdlc-coordinator/internal/store"
)

func TestServiceCreatesPersonWithNormalizedGitLabIdentity(t *testing.T) {
	service := newTestService(t)

	person, err := service.Create(context.Background(), people.CreatePersonRequest{
		DisplayName: " 测试用户乙 ",
		Email:       "USER@EXAMPLE.COM ",
		Enabled:     true,
		Identities: []people.IdentityInput{{
			Provider:       "GITLAB",
			BaseURL:        "HTTPS://GitLab.Example.Com/",
			ExternalUserID: " 1002 ",
			Username:       " User-B ",
		}},
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if person.DisplayName != "测试用户乙" || person.Email != "user@example.com" {
		t.Fatalf("unexpected normalized person: %#v", person)
	}
	if len(person.Identities) != 1 {
		t.Fatalf("expected one identity, got %#v", person.Identities)
	}
	identity := person.Identities[0]
	if identity.Provider != "gitlab" || identity.CodeHostBaseURL != "https://gitlab.example.com" || identity.ExternalUserID != "1002" || identity.Username != "user-b" {
		t.Fatalf("unexpected normalized identity: %#v", identity)
	}
}

func TestServiceRejectsDuplicateActiveGitLabIdentity(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	_, err := service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "甲", Email: "first@example.com", Enabled: true,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ExternalUserID: "7", Username: "first"}},
	})
	if err != nil {
		t.Fatalf("create first person: %v", err)
	}

	_, err = service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "乙", Email: "second@example.com", Enabled: true,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ExternalUserID: "7", Username: "second"}},
	})
	if err != people.ErrCodeHostIdentityConflict {
		t.Fatalf("expected identity conflict, got %v", err)
	}
}

func TestServiceSoftDeleteReleasesEmailAndIdentity(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	person, err := service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "甲", Email: "user@example.com", Enabled: true,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ExternalUserID: "7", Username: "user"}},
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if err := service.Delete(ctx, person.PersonID); err != nil {
		t.Fatalf("delete person: %v", err)
	}

	replacement, err := service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "乙", Email: "user@example.com", Enabled: true,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ExternalUserID: "7", Username: "user"}},
	})
	if err != nil {
		t.Fatalf("create replacement: %v", err)
	}
	if replacement.PersonID == person.PersonID {
		t.Fatalf("expected a new person identity, got %#v", replacement)
	}
}

func TestServiceUpdateCanRetainIdentityIDWhileReplacingIdentityRows(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	person, err := service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "甲", Email: "user@example.com", Enabled: true,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ExternalUserID: "7", Username: "user"}},
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	identityID := person.Identities[0].IdentityID

	updated, err := service.Update(ctx, person.PersonID, people.UpdatePersonRequest{
		DisplayName: "甲更新", Email: "user@example.com", Enabled: true,
		Identities: []people.IdentityInput{{IdentityID: identityID, Provider: "gitlab", BaseURL: "https://gitlab.example.com", ExternalUserID: "7", Username: "user"}},
	})
	if err != nil {
		t.Fatalf("update person: %v", err)
	}
	if updated.DisplayName != "甲更新" || len(updated.Identities) != 1 || updated.Identities[0].IdentityID != identityID {
		t.Fatalf("unexpected updated person: %#v", updated)
	}
}

func TestServiceDeleteIsIdempotentForAlreadyDeletedPerson(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	person, err := service.Create(ctx, people.CreatePersonRequest{DisplayName: "甲", Email: "user@example.com", Enabled: false})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if err := service.Delete(ctx, person.PersonID); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := service.Delete(ctx, person.PersonID); err != nil {
		t.Fatalf("repeated delete must be idempotent: %v", err)
	}
}

func TestServiceAllowsDisabledDraftIdentity(t *testing.T) {
	service := newTestService(t)
	person, err := service.Create(context.Background(), people.CreatePersonRequest{
		DisplayName: "甲", Email: "draft@example.com", Enabled: false,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}},
	})
	if err != nil {
		t.Fatalf("create disabled draft: %v", err)
	}
	if len(person.Identities) != 1 || person.Identities[0].Username != "" || person.Identities[0].ExternalUserID != "" {
		t.Fatalf("unexpected disabled draft: %#v", person)
	}
}

func TestServiceRejectsEnabledPersonWithIncompleteIdentity(t *testing.T) {
	service := newTestService(t)
	_, err := service.Create(context.Background(), people.CreatePersonRequest{
		DisplayName: "甲", Email: "enabled@example.com", Enabled: true,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}},
	})
	if !errors.Is(err, people.ErrIncompleteEnabledPerson) {
		t.Fatalf("expected incomplete enabled person, got %v", err)
	}
}

func TestServiceRejectsEnabledPersonWithoutIdentity(t *testing.T) {
	service := newTestService(t)
	_, err := service.Create(context.Background(), people.CreatePersonRequest{
		DisplayName: "甲", Email: "enabled@example.com", Enabled: true,
	})
	if !errors.Is(err, people.ErrIncompleteEnabledPerson) {
		t.Fatalf("expected enabled person without identity to fail, got %v", err)
	}
}

func TestServiceRejectsHalfFilledDraftIdentity(t *testing.T) {
	service := newTestService(t)
	_, err := service.Create(context.Background(), people.CreatePersonRequest{
		DisplayName: "甲", Email: "draft@example.com", Enabled: false,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com", Username: "draft"}},
	})
	if !errors.Is(err, people.ErrInvalidPerson) {
		t.Fatalf("expected half-filled draft to be invalid, got %v", err)
	}
}

func TestServiceAllowsMultipleDraftIdentitiesAtOneGitLab(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	for index, email := range []string{"first@example.com", "second@example.com"} {
		_, err := service.Create(ctx, people.CreatePersonRequest{
			DisplayName: string(rune('甲' + index)), Email: email, Enabled: false,
			Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}},
		})
		if err != nil {
			t.Fatalf("create draft %d: %v", index, err)
		}
	}
}

func TestServiceListRequiresProviderAndBaseURLOnTheSameIdentity(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	_, err := service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "甲", Email: "user@example.com", Enabled: true,
		Identities: []people.IdentityInput{
			{Provider: "gitlab", BaseURL: "https://gitlab-a.example.com", ExternalUserID: "7", Username: "user-a"},
			{Provider: "gitea", BaseURL: "https://gitea-b.example.com", ExternalUserID: "8", Username: "user-b"},
		},
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	page, err := service.List(ctx, people.PersonFilter{Provider: "gitlab", BaseURL: "https://gitea-b.example.com"})
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("filters matched different identities: %#v", page)
	}
}

func newTestService(t *testing.T) *people.Service {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repository := people.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate people: %v", err)
	}
	return people.NewService(repository)
}
