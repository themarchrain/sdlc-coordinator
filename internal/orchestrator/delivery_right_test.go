package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"

	"gorm.io/gorm"
)

// mutexCheckpoints 是并发安全的交付检查点假实现（两通道共享一份）。
type mutexCheckpoints struct {
	mu     sync.Mutex
	record *model.DeveloperDeliveryCheckpoint
}

func (c *mutexCheckpoints) RecordBaseline(_ context.Context, record *model.DeveloperDeliveryCheckpoint) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.record == nil {
		c.record = record
	}
	return nil
}

func (c *mutexCheckpoints) Load(_ context.Context, _ string) (*model.DeveloperDeliveryCheckpoint, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.record == nil {
		return nil, nil
	}
	copy := *c.record
	return &copy, nil
}

func (c *mutexCheckpoints) RecordCommit(_ context.Context, _ string, commitSHA string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.record == nil {
		return errors.New("checkpoint baseline is missing")
	}
	c.record.CommitSHA = commitSHA
	return nil
}

func (c *mutexCheckpoints) commitSHA() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.record == nil {
		return ""
	}
	return c.record.CommitSHA
}

func newDeliveryRightLeaseRepo(t *testing.T) (*repository.WorkItemLeaseRepository, *gorm.DB) {
	t.Helper()
	db, err := store.Open("file:delivery_right_" + model.NewID("t") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open lease db: %v", err)
	}
	if err := store.Migrate(db, &model.WorkItemLease{}); err != nil {
		t.Fatalf("migrate lease db: %v", err)
	}
	return repository.NewWorkItemLeaseRepository(db), db
}

func TestDeveloperDeliveryRightSerializesConcurrentChannels(t *testing.T) {
	ctx := context.Background()
	f := newResultChannelFixture("DeveloperWorker")
	leases, _ := newDeliveryRightLeaseRepo(t)
	checkpoints := &mutexCheckpoints{record: &model.DeveloperDeliveryCheckpoint{
		TaskID: f.task.TaskID, RunID: f.run.RunID, TaskType: f.task.Type,
		RepositoryURL: "https://gitlab.example/group/project.git", DeliveryBranch: f.task.DeliveryBranch,
		BaselineSHA: "baseline-sha",
	}}
	settings := ReviewerRunnerSettings{WorkspaceRoot: "D:/tmp/ws", ResolveRepository: stubRunnerRepository()}

	mcpFinalizer := &fakeDeveloperFinalizer{commit: &workspace.DeveloperCommitResult{CommitSHA: "commit-from-mcp"}}
	textFinalizer := &fakeDeveloperFinalizer{commit: &workspace.DeveloperCommitResult{CommitSHA: "commit-from-text"}}
	mcpChannel := NewResultChannelCoordinator(mcpFinalizer, &recordingResultSubmitter{}, settings, "mcp")
	mcpChannel.SetDeveloperDeliveryCheckpoints(checkpoints)
	mcpChannel.SetDeliveryExecutionRight(leases)
	textChannel := NewResultChannelCoordinator(textFinalizer, &recordingResultSubmitter{}, settings, "agent_runner")
	textChannel.SetDeveloperDeliveryCheckpoints(checkpoints)
	textChannel.SetDeliveryExecutionRight(leases)

	channels := []*ResultChannelCoordinator{mcpChannel, textChannel}
	finalizers := []*fakeDeveloperFinalizer{mcpFinalizer, textFinalizer}
	outcomes := make([]ChannelOutcome, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range channels {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			outcomes[i], errs[i] = channels[i].SubmitAgentResult(ctx, f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
		}(i)
	}
	close(start)
	wg.Wait()

	commits := mcpFinalizer.commits + textFinalizer.commits
	pushes := mcpFinalizer.pushes + textFinalizer.pushes
	if commits != 1 {
		t.Fatalf("exactly one channel must commit, got commits=%d (mcp=%d text=%d) outcomes=%+v errs=%v", commits, mcpFinalizer.commits, textFinalizer.commits, outcomes, errs)
	}
	if pushes != 1 {
		t.Fatalf("exactly one channel must push, got pushes=%d", pushes)
	}
	winner, loser := -1, -1
	for i, outcome := range outcomes {
		if errs[i] != nil {
			t.Fatalf("channel %d submit error: %v", i, errs[i])
		}
		switch outcome.Decision {
		case ChannelAccepted:
			if winner == -1 {
				winner = i
			}
		case ChannelFailed:
			if outcome.Err == nil || !strings.Contains(outcome.Err.Error(), "another coordinator instance") {
				t.Fatalf("loser failure must report the delivery right owner: err=%v", outcome.Err)
			}
			loser = i
		default:
			t.Fatalf("unexpected channel %d outcome: %#v", i, outcome)
		}
	}
	if winner == -1 || loser == -1 {
		t.Fatalf("exactly one winner and one loser required: outcomes=%+v", outcomes)
	}
	if finalizers[loser].commits != 0 || finalizers[loser].pushes != 0 {
		t.Fatalf("loser must not touch git: commits=%d pushes=%d", finalizers[loser].commits, finalizers[loser].pushes)
	}

	// 败者重试：检查点已就绪（含胜者 SHA）→ 只推同一固定 SHA，不产生第二笔提交。
	winnerSHA := checkpoints.commitSHA()
	if winnerSHA == "" {
		t.Fatal("winner must record the delivery commit sha in the checkpoint")
	}
	retry, err := channels[loser].SubmitAgentResult(ctx, f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil {
		t.Fatalf("loser retry: %v", err)
	}
	if retry.Decision != ChannelAccepted {
		t.Fatalf("checkpointed retry must converge: %#v", retry)
	}
	if finalizers[loser].commits != 0 {
		t.Fatalf("retry must not create a second commit (commits=%d)", finalizers[loser].commits)
	}
	if finalizers[loser].pushes != 1 || finalizers[loser].pushRequest.CommitSHA != winnerSHA {
		t.Fatalf("retry must push the checkpointed sha %s: %+v", winnerSHA, finalizers[loser].pushRequest)
	}
	if total := mcpFinalizer.commits + textFinalizer.commits; total != 1 {
		t.Fatalf("total commits after retry = %d, want 1", total)
	}
}

// TestDeveloperDeliveryRightTakeoverBlocksPush 取得交付权后在 commit 期间被
// 接管：push 前核验失败，绝不推送（陈旧实例不得产生新的外部可见交付）。
func TestDeveloperDeliveryRightTakeoverBlocksPush(t *testing.T) {
	ctx := context.Background()
	f := newResultChannelFixture("DeveloperWorker")
	leases, leaseDB := newDeliveryRightLeaseRepo(t)
	checkpoints := &mutexCheckpoints{record: &model.DeveloperDeliveryCheckpoint{
		TaskID: f.task.TaskID, RunID: f.run.RunID, TaskType: f.task.Type,
		RepositoryURL: "https://gitlab.example/group/project.git", DeliveryBranch: f.task.DeliveryBranch,
		BaselineSHA: "baseline-sha",
	}}
	settings := ReviewerRunnerSettings{WorkspaceRoot: "D:/tmp/ws", ResolveRepository: stubRunnerRepository()}

	finalizer := &fakeDeveloperFinalizer{commit: &workspace.DeveloperCommitResult{CommitSHA: "commit-before-takeover"}}
	// commit 期间被另一实例接管：强制租约过期后由 instance-b 抢走。
	finalizer.onCreate = func() {
		if err := forceLeaseExpiryAndTakeover(leaseDB, leases, f.run.RunID, "instance-b"); err != nil {
			t.Errorf("takeover during commit: %v", err)
		}
	}
	channel := NewResultChannelCoordinator(finalizer, &recordingResultSubmitter{}, settings, "mcp")
	channel.SetDeveloperDeliveryCheckpoints(checkpoints)
	channel.SetDeliveryExecutionRight(leases)

	outcome, err := channel.SubmitAgentResult(ctx, f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if outcome.Decision != ChannelFailed || outcome.Err == nil || !strings.Contains(outcome.Err.Error(), "lost before push") {
		t.Fatalf("takeover must block the push: %#v", outcome)
	}
	if finalizer.pushes != 0 {
		t.Fatalf("stale instance must not push after takeover (pushes=%d)", finalizer.pushes)
	}
	if finalizer.commits != 1 {
		t.Fatalf("the commit happened before the takeover (commits=%d)", finalizer.commits)
	}
}

// forceLeaseExpiryAndTakeover 把 run 的交付租约拨到过期并由指定 owner 接管。
func forceLeaseExpiryAndTakeover(db *gorm.DB, leases *repository.WorkItemLeaseRepository, runID, owner string) error {
	scope := "developer-delivery:" + runID
	// owner 形如 <instance>:<source>；接管方是另一实例的另一通道持有者。
	takeoverOwner := owner + ":mcp"
	now := time.Now().UTC().Add(-time.Hour)
	if err := db.Exec("UPDATE work_item_leases SET lease_until = ? WHERE scope = ?", now, scope).Error; err != nil {
		return err
	}
	_, won, err := leases.Acquire(context.Background(), scope, takeoverOwner, "", 5*time.Minute)
	if err != nil {
		return err
	}
	if !won {
		return errors.New("takeover did not win")
	}
	return nil
}

// TestDeveloperDeliveryCommitResponseLostStaysConservative commit 已产生但响应
// 丢失（可信记录缺失）：重试不得凭本地状态认领或再提交第二笔，保守拒绝。
func TestDeveloperDeliveryCommitResponseLostStaysConservative(t *testing.T) {
	ctx := context.Background()
	f := newResultChannelFixture("DeveloperWorker")
	leases, _ := newDeliveryRightLeaseRepo(t)
	checkpoints := &mutexCheckpoints{record: &model.DeveloperDeliveryCheckpoint{
		TaskID: f.task.TaskID, RunID: f.run.RunID, TaskType: f.task.Type,
		RepositoryURL: "https://gitlab.example/group/project.git", DeliveryBranch: f.task.DeliveryBranch,
		BaselineSHA: "baseline-sha",
	}}
	settings := ReviewerRunnerSettings{WorkspaceRoot: "D:/tmp/ws", ResolveRepository: stubRunnerRepository()}

	// 第一次：commit 实际发生（工作区出现交付提交）但响应丢失。
	lostFinalizer := &fakeDeveloperFinalizer{commitErr: errors.New("connection reset by peer")}
	channel := NewResultChannelCoordinator(lostFinalizer, &recordingResultSubmitter{}, settings, "mcp")
	channel.SetDeveloperDeliveryCheckpoints(checkpoints)
	channel.SetDeliveryExecutionRight(leases)
	outcome, err := channel.SubmitAgentResult(ctx, f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if outcome.Decision != ChannelRejected && outcome.Decision != ChannelFailed {
		t.Fatalf("lost commit response must not be accepted: %#v", outcome)
	}
	if checkpoints.commitSHA() != "" {
		t.Fatalf("lost response must not record a checkpoint sha: %q", checkpoints.commitSHA())
	}

	// 第二次：本地工作区已有交付提交（保守拒绝，不产生第二笔）。
	retryFinalizer := &fakeDeveloperFinalizer{commitErr: workspace.ErrWorkspaceConflict}
	retryChannel := NewResultChannelCoordinator(retryFinalizer, &recordingResultSubmitter{}, settings, "mcp")
	retryChannel.SetDeveloperDeliveryCheckpoints(checkpoints)
	retryChannel.SetDeliveryExecutionRight(leases)
	retry, err := retryChannel.SubmitAgentResult(ctx, f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil {
		t.Fatalf("retry submit: %v", err)
	}
	if retry.Decision == ChannelAccepted {
		t.Fatalf("retry must stay conservative without a trusted sha: %#v", retry)
	}
	if retryFinalizer.commits != 1 || retryFinalizer.pushes != 0 {
		t.Fatalf("retry must not push and must not bypass the refusal: commits=%d pushes=%d", retryFinalizer.commits, retryFinalizer.pushes)
	}
}
