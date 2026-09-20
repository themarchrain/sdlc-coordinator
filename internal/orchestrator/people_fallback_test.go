package orchestrator

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
)

func TestReviewerMentionsFallBackToPlatformWhenRepoEmpty(t *testing.T) {
	repoID := "repo_a"
	orch := New(nil, nil,
		WithReviewerIMMentions([]string{"platform-reviewer@example.com"}),
		WithRepositoryLookup(staticRepositoryLookup{id: repoID, status: "enabled"}),
	)
	task := &model.Task{RepositoryID: &repoID}

	got := orch.reviewerMentionsFor(context.Background(), task)
	if len(got) != 1 || got[0] != "platform-reviewer@example.com" {
		t.Fatalf("empty repo mentions must fall back to platform: %#v", got)
	}
}

type staticRepositoryLookup struct {
	id     string
	status string
	repo   *registeredrepo.RegisteredRepository
}

func (lookup staticRepositoryLookup) Get(_ context.Context, repositoryID string) (*registeredrepo.RegisteredRepository, error) {
	if lookup.repo != nil {
		return lookup.repo, nil
	}
	id := lookup.id
	if id == "" {
		id = repositoryID
	}
	return &registeredrepo.RegisteredRepository{RepositoryID: id, Status: registeredrepo.StatusEnabled}, nil
}

func TestReviewerMentionsFollowProviderChanges(t *testing.T) {
	repoID := "repo_a"
	current := []string{"alice"}
	orch := New(nil, nil,
		WithReviewerIMMentionsProvider(func(context.Context) []string { return current }),
		WithRepositoryLookup(staticRepositoryLookup{id: repoID, status: "enabled"}),
	)
	task := &model.Task{RepositoryID: &repoID}

	if got := orch.reviewerMentionsFor(context.Background(), task); len(got) != 1 || got[0] != "alice" {
		t.Fatalf("mentions must come from provider: %#v", got)
	}
	current = []string{"bob"}
	if got := orch.reviewerMentionsFor(context.Background(), task); len(got) != 1 || got[0] != "bob" {
		t.Fatalf("mentions must track provider changes: %#v", got)
	}
}

func TestReviewerMentionsProviderAbsentFallsBackToEmpty(t *testing.T) {
	orch := New(nil, nil)
	if got := orch.reviewerMentionsFor(context.Background(), nil); len(got) != 0 {
		t.Fatalf("no provider must yield empty mentions: %#v", got)
	}
}
