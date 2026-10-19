package requirements

import (
	"context"
	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/store/storetest"
	"sync"
	"testing"
)

// TestPublishRequirementDualInstanceSingleCreate 双实例同时发布同一需求：
// 租约执行权保证远端 CreateIssue 恰好一次,败者本轮 skipped 且不落 failed。
func TestPublishRequirementDualInstanceSingleCreate(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "req_publish")

	ctx := context.Background()
	client := &fakeIssueClient{createIssue: &codehost.Issue{IID: 7, WebURL: "https://gitlab.example/group/demo/-/issues/7"}}

	services := make([]*Service, 2)
	for i := range services {
		db, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open instance %d: %v", i, err)
		}
		defer func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}()
		if err := store.Migrate(db, &model.WorkItemLease{}); err != nil {
			t.Fatalf("migrate leases: %v", err)
		}
		requirementStore := NewStore(db)
		if err := requirementStore.Migrate(); err != nil {
			t.Fatalf("migrate requirements: %v", err)
		}
		repoStore := registeredrepo.NewStore(db)
		if err := repoStore.Migrate(); err != nil {
			t.Fatalf("migrate repos: %v", err)
		}
		if i == 0 {
			if err := repoStore.Create(ctx, &registeredrepo.RegisteredRepository{
				RepositoryID: "repo-1", DisplayName: "Demo", PathWithNamespace: "group/demo",
				Token: "tok-1", Status: registeredrepo.StatusEnabled,
			}); err != nil {
				t.Fatalf("seed repo: %v", err)
			}
		}
		service := NewService(requirementStore, repoStore, func(*registeredrepo.RegisteredRepository) ClientFactory { return client })
		service.WithWorkItemLeases(repository.NewWorkItemLeaseRepository(db), "test-req-instance-"+string(rune('A'+i)))
		services[i] = service
	}

	item, err := services[0].Create(ctx, CreateRequest{
		Title: "并发发布", Description: "双实例竞争", RepositoryID: "repo-1",
	})
	if err != nil {
		t.Fatalf("create requirement: %v", err)
	}

	outcomes := make([]publishOutcome, 2)
	outErrs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range services {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			loaded, loadErr := services[i].store.Get(ctx, item.RequirementID)
			if loadErr != nil {
				outErrs[i] = loadErr
				return
			}
			outcomes[i], outErrs[i] = services[i].publishRequirement(ctx, loaded)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range outErrs {
		if err != nil {
			t.Fatalf("instance %d: %v", i, err)
		}
	}
	if client.createCalls != 1 {
		t.Fatalf("CreateIssue called %d times across instances, want exactly 1", client.createCalls)
	}
	wins := 0
	for i, outcome := range outcomes {
		switch outcome {
		case publishCreated, publishRecovered:
			wins++
		case publishSkipped:
			// 败者:合法跳过。
		default:
			t.Fatalf("instance %d unexpected outcome %d", i, outcome)
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one instance may publish, got %d", wins)
	}

	final, err := services[0].store.Get(ctx, item.RequirementID)
	if err != nil {
		t.Fatalf("reload requirement: %v", err)
	}
	if final.Status != StatusPublished || final.IssueIID != 7 {
		t.Fatalf("requirement = %s iid %d, want published/7", final.Status, final.IssueIID)
	}
}
