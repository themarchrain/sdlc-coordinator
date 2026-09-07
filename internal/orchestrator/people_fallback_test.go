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
