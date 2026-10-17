package runnerpool

import (
	"context"
	"sync"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

func TestRoutersSharePoolSlotsAcrossMySQLConnections(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "runner_pool_slots")
	dbA, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	dbB, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := dbA.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := dbB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	repoA := controlplane.NewRepository(dbA)
	if err := repoA.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repoA.CreateTenant(ctx, &controlplane.Tenant{ID: 7, Slug: "runner-pool", DisplayName: "Runner Pool"}); err != nil {
		t.Fatal(err)
	}
	serviceA := controlplane.NewService(repoA)
	if err := serviceA.ReplaceRunnerAssignments(ctx, nil, 7, []string{"http://runner-a", "http://runner-b"}); err != nil {
		t.Fatal(err)
	}
	serviceB := controlplane.NewService(controlplane.NewRepository(dbB))
	routers := []*Router{
		newTestRouter(&fakeTaskTenants{tenant: 7}, NewControlPlanePoolSource(serviceA, nil)),
		newTestRouter(&fakeTaskTenants{tenant: 7}, NewControlPlanePoolSource(serviceB, nil)),
	}
	type selected struct {
		endpoint string
		err      error
	}
	ready := make(chan struct{})
	results := make(chan selected, 2)
	var workers sync.WaitGroup
	for i, router := range routers {
		workers.Add(1)
		go func(i int, router *Router) {
			defer workers.Done()
			<-ready
			attempt, err := router.AttemptForTask(ctx, &model.Task{TaskID: "task-" + string(rune('a'+i))})
			results <- selected{endpoint: attempt.Endpoint, err: err}
		}(i, router)
	}
	close(ready)
	workers.Wait()
	close(results)
	seen := map[string]int{}
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		seen[result.endpoint]++
	}
	if seen["http://runner-a"] != 1 || seen["http://runner-b"] != 1 {
		t.Fatalf("two Coordinators did not split first tasks across Runners: %v", seen)
	}
	if err := serviceA.ReplaceRunnerAssignments(ctx, nil, 7, []string{"http://runner-a", "http://runner-b", "http://runner-c"}); err != nil {
		t.Fatal(err)
	}
	seen = map[string]int{}
	for i := 0; i < 3; i++ {
		attempt, err := routers[i%2].AttemptForTask(ctx, &model.Task{TaskID: "expanded"})
		if err != nil {
			t.Fatal(err)
		}
		seen[attempt.Endpoint]++
	}
	if len(seen) != 3 {
		t.Fatalf("new Runner did not receive work after expansion: %v", seen)
	}
	if err := serviceA.ReplaceRunnerAssignments(ctx, nil, 7, []string{"http://runner-a", "http://runner-c"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		attempt, err := routers[i%2].AttemptForTask(ctx, &model.Task{TaskID: "reduced"})
		if err != nil || attempt.Endpoint == "http://runner-b" {
			t.Fatalf("removed Runner received a new task: endpoint=%q err=%v", attempt.Endpoint, err)
		}
	}
}
