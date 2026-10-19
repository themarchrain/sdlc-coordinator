package workspaceproto

import "testing"

func TestDeliveryPayloadHashSurvivesLeaseTakeover(t *testing.T) {
	commit := CommitRequest{
		OperationID: "commit-run", TenantID: 1, TaskID: "task", RunID: "run",
		Message: "deliver", RepositoryURL: "https://example.test/repo.git",
		BaselineSHA: "baseline", Owner: "coordinator-a", Generation: 1,
	}
	push := PushRequest{
		OperationID: "push-run", TenantID: 1, TaskID: "task", RunID: "run",
		RepositoryURL: "https://example.test/repo.git", CommitSHA: "commit",
		BaselineSHA: "baseline", Owner: "coordinator-a", Generation: 1,
	}
	for name, pair := range map[string][2]any{
		"commit": {commit, func() CommitRequest { next := commit; next.Owner = "coordinator-b"; next.Generation = 2; return next }()},
		"push":   {push, func() PushRequest { next := push; next.Owner = "coordinator-b"; next.Generation = 2; return next }()},
	} {
		t.Run(name, func(t *testing.T) {
			oldHash, err := PayloadHash(pair[0])
			if err != nil {
				t.Fatal(err)
			}
			newHash, err := PayloadHash(pair[1])
			if err != nil {
				t.Fatal(err)
			}
			if oldHash != newHash {
				t.Fatalf("lease takeover changed the fixed operation payload hash: %s != %s", oldHash, newHash)
			}
		})
	}
}
