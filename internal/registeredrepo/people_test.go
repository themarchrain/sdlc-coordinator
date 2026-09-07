package registeredrepo

import "testing"

func TestResolvePeopleFallsBackToPlatformWhenRepoFieldsEmpty(t *testing.T) {
	repo := &RegisteredRepository{}
	platform := PeopleSettings{
		ReviewerMentions: "reviewer@example.com",
	}

	got := ResolvePeople(repo, platform)
	if got != platform {
		t.Fatalf("empty repo fields must fall back to platform defaults: %#v", got)
	}
}

func TestResolvePeoplePrefersRepositoryFields(t *testing.T) {
	repo := &RegisteredRepository{
		ReviewerMentions: "repo-reviewer@example.com",
	}
	platform := PeopleSettings{
		ReviewerMentions: "reviewer@example.com",
	}

	got := ResolvePeople(repo, platform)
	if got.ReviewerMentions != repo.ReviewerMentions {
		t.Fatalf("repository reviewer mentions should win: %#v", got)
	}
}

func TestResolvePeopleFallsBackPerField(t *testing.T) {
	repo := &RegisteredRepository{ReviewerMentions: "repo-reviewer@example.com"}
	platform := PeopleSettings{
		ReviewerMentions: "reviewer@example.com",
	}

	got := ResolvePeople(repo, platform)
	if got.ReviewerMentions != repo.ReviewerMentions {
		t.Fatalf("non-empty repo reviewer mentions should win: %#v", got)
	}
}

func TestResolvePeopleNilRepositoryUsesPlatform(t *testing.T) {
	platform := PeopleSettings{ReviewerMentions: "reviewer@example.com"}
	got := ResolvePeople(nil, platform)
	if got != platform {
		t.Fatalf("nil repo must use platform defaults: %#v", got)
	}
}
