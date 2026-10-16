package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/instance"
	"sdlc-coordinator/internal/model"
	orchestrator "sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/store/storetest"
	"sdlc-coordinator/internal/workspace"
	"sdlc-coordinator/internal/workspaceproto"

	"gorm.io/gorm"
)

func TestRemoteDeliveryTakeoverFencesStaleCoordinatorOnMySQL(t *testing.T) {
	if os.Getenv("CI") != "" && strings.TrimSpace(os.Getenv(storetest.MySQLDSNEnv)) == "" {
		t.Fatalf("CI must configure %s so the MySQL takeover integration test cannot silently skip", storetest.MySQLDSNEnv)
	}
	control, resolver, _ := mysqlTenantWorld(t)
	control.SetTenantProvisioner(&tenantStoreProvisioner{resolver: resolver})
	tenant, err := control.CreateTenant(context.Background(), nil, controlplane.TenantInput{
		Slug: "lease-takeover", DisplayName: "Lease takeover",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	handle, err := resolver.Handle(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("open tenant database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := handle.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	db := handle.DB
	baseURL, pathWithNamespace, originDir, seedSHA := newRemoteDeliveryOrigin(t)
	repositoryID := "repo-mysql-delivery-takeover"
	repoRow := &registeredrepo.RegisteredRepository{
		RepositoryID: repositoryID, Provider: "github", BaseURL: baseURL,
		PathWithNamespace: pathWithNamespace, Status: registeredrepo.StatusEnabled,
		GitAuthorName: "AI Developer", GitAuthorEmail: "ai-developer@localhost",
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	const taskID = "task-mysql-delivery-takeover"
	const runID = "run-mysql-delivery-takeover"
	runIDPointer := runID
	task := &model.Task{
		TaskID: taskID, DedupeKey: "dedupe-" + taskID, Type: "develop_from_issue", Role: "DeveloperWorker",
		RepositoryID: &repositoryID,
		TargetRepo:   "group/project", SourceRepo: "group/project", DeliveryBranch: "agent/" + taskID,
		Status: model.TaskStatusRunning, CurrentRunID: &runIDPointer,
		ContextJSON: `{"project":{"path":"group/project"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"main","access":"writable"}]}`,
	}
	if err := tasks.Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: runID, TaskID: taskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	settings := orchestrator.ReviewerRunnerSettings{
		WorkspaceRoot: "unused-in-remote-mode",
		ResolveRepository: func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
			return repoRow, nil
		},
	}
	checkpoints := repository.NewDeveloperDeliveryCheckpointRepository(db)
	leasesA := repository.NewWorkItemLeaseRepository(db)
	leasesB := repository.NewWorkItemLeaseRepository(mustOpenMySQLTestDB(t, storetest.RequireMySQL(t), tenant.DMLocator))
	const token = "mysql-delivery-takeover-internal-token"
	authorizer := &tenantRunnerAuthorizer{control: control, resolver: resolver}
	authorizationCounts := map[string]map[string]int{}
	var commitOwner string
	var commitGeneration uint64
	var authMu sync.Mutex
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{})
	if err != nil {
		t.Fatalf("runner service: %v", err)
	}
	ops, err := runner.NewWorkspaceOps(runner.WorkspaceOpsConfig{
		DataRoot: t.TempDir(),
		Authorize: func(ctx context.Context, request workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
			decision, authErr := authorizer.AuthorizeRunnerWorkspaceOperation(ctx, request)
			if authErr == nil && (request.Kind == workspaceproto.KindDeveloperCommit || request.Kind == workspaceproto.KindDeveloperPush) {
				authMu.Lock()
				if authorizationCounts[request.Kind] == nil {
					authorizationCounts[request.Kind] = map[string]int{}
				}
				verdict := "denied"
				if decision.Authorized {
					verdict = "authorized"
				}
				authorizationCounts[request.Kind][verdict]++
				if request.Kind == workspaceproto.KindDeveloperCommit && decision.Authorized {
					commitOwner, commitGeneration = request.Owner, request.Generation
				}
				authMu.Unlock()
			}
			return decision, authErr
		},
	})
	if err != nil {
		t.Fatalf("runner workspace operations: %v", err)
	}
	server := httptest.NewServer(runner.NewHTTPHandler(service, runner.WithWorkspaceOperations(ops, token)))
	t.Cleanup(server.Close)
	remote := agentrunner.NewRemoteWorkspace(server.URL, token, tenant.ID, nil)

	manifest, err := agentrunner.NewRemoteDeveloperPreparer(remote).Prepare(context.Background(), workspace.DeveloperRequest{
		TaskID: taskID, RunID: runID, TaskType: task.Type, Project: "group/project",
		RepositoryURL: repoRow.CloneURL(), TargetBranch: "main", DeliveryBranch: task.DeliveryBranch,
		GitAuthorName: repoRow.GitAuthorName, GitAuthorEmail: repoRow.GitAuthorEmail,
	})
	if err != nil {
		t.Fatalf("prepare remote workspace: %v", err)
	}
	if manifest == nil || manifest.Repository.InitialHeadSHA != seedSHA {
		t.Fatalf("workspace baseline = %+v, want %s", manifest, seedSHA)
	}
	if err := checkpoints.RecordBaseline(context.Background(), &model.DeveloperDeliveryCheckpoint{
		TaskID: taskID, RunID: runID, TaskType: task.Type,
		RepositoryURL: repoRow.CloneURL(), DeliveryBranch: task.DeliveryBranch, BaselineSHA: seedSHA,
	}); err != nil {
		t.Fatalf("record delivery baseline: %v", err)
	}
	repoRoot := filepath.Join(ops.WorkspaceRoot(), "runs", runID, "repo")
	if err := os.WriteFile(filepath.Join(repoRoot, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write runner-side change: %v", err)
	}

	ownerA := instance.ID() + ":mcp"
	ownerB := instance.ID() + ":text"
	leaseScope := "developer-delivery:" + runID
	var takeoverLease model.WorkItemLease
	transport := &takeoverAfterCommitTransport{
		base: http.DefaultTransport,
		takeover: func() error {
			if commitOwner != ownerA || commitGeneration == 0 {
				return errors.New("commit request did not carry coordinator A's execution right")
			}
			if err := db.Exec("UPDATE work_item_leases SET lease_until = DATE_SUB(NOW(6), INTERVAL 1 SECOND) WHERE scope = ?", leaseScope).Error; err != nil {
				return err
			}
			var won bool
			var acquireErr error
			takeoverLease, won, acquireErr = leasesB.Acquire(context.Background(), leaseScope, ownerB, "developer-delivery", 5*time.Minute)
			if acquireErr != nil {
				return acquireErr
			}
			if !won || takeoverLease.Generation != commitGeneration+1 {
				return errors.New("coordinator B did not take over the expired lease at the next generation")
			}
			return nil
		},
	}
	clientA := &http.Client{Timeout: 2 * time.Minute, Transport: transport}
	remoteA := agentrunner.NewRemoteWorkspace(server.URL, token, tenant.ID, clientA)
	submitterA := &recordingSubmitter{}
	payload := strings.ReplaceAll(string(remoteDeliveryPayload()), "task-remote-delivery", taskID)
	payload = strings.ReplaceAll(payload, "run-remote-delivery", runID)
	channelA := newMySQLDeliveryChannel(remoteA, settings, checkpoints, leasesA, "mcp", submitterA)
	outcomeA, err := channelA.SubmitAgentResult(context.Background(), task, run, results.SubmitDeveloperResultTool, []byte(payload))
	if err != nil {
		t.Fatalf("coordinator A submit: %v", err)
	}
	if transport.takeoverErr != nil {
		t.Fatalf("coordinator B takeover after runner commit: %v", transport.takeoverErr)
	}
	if outcomeA.Decision != orchestrator.ChannelFailed || outcomeA.Err == nil || !strings.Contains(outcomeA.Err.Error(), "right was lost before push") {
		t.Fatalf("coordinator A must stop before push after takeover, got %+v", outcomeA)
	}
	if submitterA.calls != 0 {
		t.Fatalf("coordinator A submitted a result after losing delivery ownership (%d calls)", submitterA.calls)
	}
	checkpoint, err := checkpoints.Load(context.Background(), runID)
	if err != nil || checkpoint == nil || checkpoint.CommitSHA == "" {
		t.Fatalf("runner commit checkpoint after takeover: %+v err=%v", checkpoint, err)
	}
	localCommits := strings.Fields(strings.TrimSpace(deliveryGit(t, repoRoot, "rev-list", "HEAD")))
	if len(localCommits) != 2 {
		t.Fatalf("Runner workspace has %d commits after A's accepted operation, want seed + one delivery commit", len(localCommits))
	}
	deliveryRef := strings.TrimSpace(deliveryGit(t, originDir, "for-each-ref", "--format=%(refname)", "refs/heads/"+task.DeliveryBranch))
	if deliveryRef != "" {
		t.Fatalf("delivery branch exists before current owner pushed: %s", deliveryRef)
	}
	remoteHead := strings.TrimSpace(deliveryGit(t, originDir, "rev-parse", "refs/heads/main"))
	if remoteHead != seedSHA {
		t.Fatalf("main advanced before current owner pushed: got %s want baseline %s", remoteHead, seedSHA)
	}

	stalePush := workspace.DeveloperPushRequest{
		TaskID: taskID, RunID: runID, TaskType: task.Type, RepositoryURL: repoRow.CloneURL(),
		TargetBranch: "main", DeliveryBranch: task.DeliveryBranch,
		CommitSHA: checkpoint.CommitSHA, BaselineSHA: checkpoint.BaselineSHA,
		DeliveryOwner: ownerA, DeliveryGeneration: commitGeneration,
	}
	if _, err := remote.PushDelivery(context.Background(), stalePush); err == nil {
		t.Fatal("Runner accepted coordinator A's new push request after B took ownership")
	}

	submitterB := &recordingSubmitter{}
	channelB := newMySQLDeliveryChannel(remote, settings, checkpoints, leasesB, "text", submitterB)
	outcomeB, err := channelB.SubmitAgentResult(context.Background(), task, run, results.SubmitDeveloperResultTool, []byte(payload))
	if err != nil {
		t.Fatalf("coordinator B submit: %v", err)
	}
	if outcomeB.Decision != orchestrator.ChannelAccepted {
		t.Fatalf("coordinator B must push the checkpointed Runner commit and converge: %+v", outcomeB)
	}
	if submitterB.calls != 1 {
		t.Fatalf("coordinator B submitted %d results, want exactly one", submitterB.calls)
	}
	checkpoint, err = checkpoints.Load(context.Background(), runID)
	if err != nil || checkpoint == nil || checkpoint.CommitSHA == "" {
		t.Fatalf("final delivery checkpoint: %+v err=%v", checkpoint, err)
	}
	if checkpoint.CommitSHA != localCommits[0] && checkpoint.CommitSHA != localCommits[1] {
		t.Fatalf("checkpoint SHA %s is not in Runner history %v", checkpoint.CommitSHA, localCommits)
	}
	remoteHead = strings.TrimSpace(deliveryGit(t, originDir, "rev-parse", "refs/heads/"+task.DeliveryBranch))
	if remoteHead != checkpoint.CommitSHA {
		t.Fatalf("remote head %s differs from the single checkpointed SHA %s", remoteHead, checkpoint.CommitSHA)
	}
	remoteCommits := strings.Fields(strings.TrimSpace(deliveryGit(t, originDir, "rev-list", "refs/heads/"+task.DeliveryBranch)))
	if len(remoteCommits) != 2 {
		t.Fatalf("remote branch has %d commits, want one delivery commit after takeover", len(remoteCommits))
	}

	authMu.Lock()
	commitAuthorizationChecks := authorizationCounts[workspaceproto.KindDeveloperCommit]["authorized"]
	pushAuthorizationChecks := authorizationCounts[workspaceproto.KindDeveloperPush]["authorized"]
	pushAuthorizationDenials := authorizationCounts[workspaceproto.KindDeveloperPush]["denied"]
	authMu.Unlock()
	if commitAuthorizationChecks != 2 || pushAuthorizationChecks != 2 || pushAuthorizationDenials != 1 {
		t.Fatalf("authorization check counts commit_allowed=%d push_allowed=%d push_denied=%d, want 2/2/1", commitAuthorizationChecks, pushAuthorizationChecks, pushAuthorizationDenials)
	}
}

func newMySQLDeliveryChannel(
	remote *agentrunner.RemoteWorkspace,
	settings orchestrator.ReviewerRunnerSettings,
	checkpoints *repository.DeveloperDeliveryCheckpointRepository,
	leases *repository.WorkItemLeaseRepository,
	source string,
	submitter *recordingSubmitter,
) *orchestrator.ResultChannelCoordinator {
	channel := orchestrator.NewResultChannelCoordinator(remote, submitter, settings, source)
	channel.SetDeveloperDeliveryCheckpoints(checkpoints)
	channel.SetDeliveryExecutionRight(leases)
	return channel
}

type takeoverAfterCommitTransport struct {
	base        http.RoundTripper
	takeover    func() error
	once        sync.Once
	takeoverErr error
}

func (t *takeoverAfterCommitTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err == nil && response != nil && request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/internal/workspace/commit") && response.StatusCode == http.StatusOK {
		t.once.Do(func() { t.takeoverErr = t.takeover() })
	}
	return response, err
}

func mustOpenMySQLTestDB(t *testing.T, adminDSN, locator string) *gorm.DB {
	t.Helper()
	parsed, err := store.ParseDSN(adminDSN)
	if err != nil {
		t.Fatalf("parse MySQL admin DSN: %v", err)
	}
	database := strings.TrimPrefix(locator, "mysql://")
	if !strings.HasPrefix(database, parsed.Host+"/") {
		t.Fatalf("tenant locator %q does not match MySQL host %q", locator, parsed.Host)
	}
	parsed = parsed.WithDatabase(strings.TrimPrefix(database, parsed.Host+"/"))
	db, err := store.OpenSpec(parsed)
	if err != nil {
		t.Fatalf("open second MySQL connection: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
