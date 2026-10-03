package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
)

// 场景：MCP 通道已把 run 终态化为 succeeded，此后 runner 侧执行也成功结束，
// 协调器 reconcile 拉到文本结果——必须静默吸收（只更新执行投影），不得再走
// 文本提交，也不得把不一致文本升级为 human_required。
func TestReviewerReconcileAbsorbsTextResultAfterMCPFinalizedRun(t *testing.T) {
	fixture := newReviewerRunnerFixture(t)
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start reviewer execution: %v", err)
	}
	runRecord, err := fixture.runs.GetByRunID(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	runRecord.Status = model.RunStatusSucceeded
	if err := fixture.runs.Update(context.Background(), runRecord); err != nil {
		t.Fatalf("finalize run as mcp would: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: json.RawMessage(`{"task_id":"task-reviewer-runner","run_id":"run-reviewer-runner","verdict":"approved","summary":"late text result","findings":[],"test_summary":"x","comment_body":"y"}`),
	}

	observed, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reconcile absorbed run: %v", err)
	}
	if !observed.Absorbed {
		t.Fatalf("result must be absorbed, got %#v", observed)
	}
	if observed.AbsorbReason == "" {
		t.Fatal("absorb reason must be recorded for tick logs")
	}
	if observed.Submission != nil || fixture.submitter.calls != 0 {
		t.Fatalf("absorbed text result must not be submitted: calls=%d", fixture.submitter.calls)
	}
}

// MCP 终态后 CLI 崩溃/失败：不得把已 succeeded 的 run 打回 failed。
func TestReviewerReconcileAbsorbsExecutionFailureAfterMCPFinalizedRun(t *testing.T) {
	fixture := newReviewerRunnerFixture(t)
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start reviewer execution: %v", err)
	}
	runRecord, _ := fixture.runs.GetByRunID(context.Background(), fixture.run.RunID)
	runRecord.Status = model.RunStatusSucceeded
	if err := fixture.runs.Update(context.Background(), runRecord); err != nil {
		t.Fatalf("finalize run: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusFailed
	fixture.runner.execution.ErrorCode = "provider_failed"

	observed, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reconcile failed execution on finalized run: %v", err)
	}
	if !observed.Absorbed {
		t.Fatalf("failure observation on finalized run must be absorbed: %#v", observed)
	}
	runRecord, _ = fixture.runs.GetByRunID(context.Background(), fixture.run.RunID)
	if runRecord.Status != model.RunStatusSucceeded {
		t.Fatalf("finalized run must keep its status, got %s", runRecord.Status)
	}
	task, _ := fixture.tasks.GetByTaskID(context.Background(), fixture.task.TaskID)
	if task.Status == model.TaskStatusHumanRequired || task.Status == model.TaskStatusBlocked {
		t.Fatalf("CLI failure must not escalate a finalized run's task, got %s", task.Status)
	}
}

// 收尾顺序的另一面：orchestrator 层 dispatch 对 absorbed 的 succeeded 执行
// 不得要求 Submission 非空（静默吸收没有提交）。
func TestDeveloperDispatchAcceptsAbsorbedSucceededExecution(t *testing.T) {
	fixture := newDeveloperRunnerFixture(t, "develop_from_issue")
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start developer execution: %v", err)
	}
	runRecord, _ := fixture.runs.GetByRunID(context.Background(), fixture.run.RunID)
	runRecord.Status = model.RunStatusSucceeded
	if err := fixture.runs.Update(context.Background(), runRecord); err != nil {
		t.Fatalf("finalize run: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: json.RawMessage(`{"task_id":"task-developer-runner","run_id":"run-developer-runner","outcome":"fix_failed","summary":"late","blocker_reason":"late text"}`),
	}
	orch := New(fixture.tasks, fixture.runs, WithDeveloperRunner(fixture.coordinator, ReviewerRunnerSettings{WorkspaceRoot: fixture.root, ResolveRepository: stubRunnerRepository()}))

	observed, err := orch.ReconcileDeveloperRunner(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reconcile absorbed developer run: %v", err)
	}
	if !observed.Absorbed {
		t.Fatalf("developer text result must be absorbed: %#v", observed)
	}
	if fixture.submitter.calls != 0 {
		t.Fatalf("absorbed result must not submit: %d", fixture.submitter.calls)
	}
}

// running 状态不回归：正常文本通道照旧提交。
func TestReviewerReconcileStillSubmitsWhileRunRunning(t *testing.T) {
	fixture := newReviewerRunnerFixture(t)
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start reviewer execution: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: json.RawMessage(`{"task_id":"task-reviewer-runner","run_id":"run-reviewer-runner","verdict":"approved","summary":"fine","findings":[],"test_summary":"not run","comment_body":"AI Review 结论：建议合并。"}`),
	}
	observed, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reconcile running reviewer execution: %v", err)
	}
	if observed.Absorbed || observed.Submission == nil {
		t.Fatalf("running run must process text results normally: %#v", observed)
	}
}

// MCP 首提终态化 run 后，tick 必须对「run 已终态但执行投影
// 仍活跃」的 run 补一次观察收尾投影，否则投影永久滞留 running、artifact
// 清理被跳过并最终饿死清理批次。
func TestTickClosesExecutionProjectionForMCPFinalizedRuns(t *testing.T) {
	fixture := newReviewerRunnerFixture(t)
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start reviewer execution: %v", err)
	}
	// MCP 首提：run 行终态化（succeeded），执行投影仍 running。
	// ExternalSystem 由真实派发写入（developer_runner_dispatch.go:34），夹具补齐。
	runRecord, _ := fixture.runs.GetByRunID(context.Background(), fixture.run.RunID)
	runRecord.Status = model.RunStatusSucceeded
	runRecord.ExternalSystem = "agent-runner"
	if err := fixture.runs.Update(context.Background(), runRecord); err != nil {
		t.Fatalf("finalize run: %v", err)
	}
	// CLI 随后正常退出。
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: json.RawMessage(`{"task_id":"task-reviewer-runner","run_id":"run-reviewer-runner","verdict":"approved","summary":"late","findings":[],"test_summary":"x","comment_body":"y"}`),
	}

	orch := New(fixture.tasks, fixture.runs, WithReviewerRunner(fixture.coordinator, ReviewerRunnerSettings{WorkspaceRoot: fixture.root}))
	if _, err := orch.Tick(context.Background(), TickRequest{PollLimit: 10}); err != nil {
		t.Fatalf("tick: %v", err)
	}
	execution, err := fixture.coordinator.core.executions.GetByRunID(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("load execution: %v", err)
	}
	if execution.Status != model.AgentExecutionStatusSucceeded {
		t.Fatalf("execution projection must close after the sweep, got %#v", execution)
	}
	storedRun, _ := fixture.runs.GetByRunID(context.Background(), fixture.run.RunID)
	if storedRun.Status != model.RunStatusSucceeded {
		t.Fatalf("sweep must not disturb the terminal run, got %s", storedRun.Status)
	}
	task, _ := fixture.tasks.GetByTaskID(context.Background(), fixture.task.TaskID)
	if task.Status == model.TaskStatusHumanRequired || task.Status == model.TaskStatusBlocked {
		t.Fatalf("sweep must not escalate, got %s", task.Status)
	}
}

// Provider 生成的交付标记在 run 已终态时只被吸收:不得再次提交业务结果,
// 写回与通知路径也不得被触发(它们只挂在 Submission 上)。
func TestReviewerReconcileAbsorbsDeliveryMarkerAfterMCPFinalizedRun(t *testing.T) {
	fixture := newReviewerRunnerFixture(t)
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start reviewer execution: %v", err)
	}
	runRecord, _ := fixture.runs.GetByRunID(context.Background(), fixture.run.RunID)
	runRecord.Status = model.RunStatusSucceeded
	if err := fixture.runs.Update(context.Background(), runRecord); err != nil {
		t.Fatalf("finalize run as mcp would: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: runner.NewDeliveryMarker(fixture.task.TaskID, fixture.run.RunID),
	}

	observed, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reconcile absorbed marker: %v", err)
	}
	if !observed.Absorbed {
		t.Fatalf("delivery marker on a terminal run must be absorbed: %#v", observed)
	}
	if observed.Submission != nil || fixture.submitter.calls != 0 {
		t.Fatalf("delivery marker must never be submitted as a business result: calls=%d", fixture.submitter.calls)
	}
}

// run 仍未终态时出现交付标记属于跨通道时序不一致:必须显式失败等待对账,
// 绝不能把标记当作业务 JSON 送进结果通道。
func TestReviewerReconcileRefusesDeliveryMarkerWhileRunRunning(t *testing.T) {
	fixture := newReviewerRunnerFixture(t)
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start reviewer execution: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: runner.NewDeliveryMarker(fixture.task.TaskID, fixture.run.RunID),
	}

	_, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err == nil || !strings.Contains(err.Error(), "delivery marker") {
		t.Fatalf("delivery marker on a running run must fail explicitly, got %v", err)
	}
	if runnerResultNeedsConvergence(err) {
		t.Fatalf("delivery marker must wait for reconciliation instead of converging the run: %v", err)
	}
	if fixture.submitter.calls != 0 {
		t.Fatalf("delivery marker must never reach SubmitAgentResult: calls=%d", fixture.submitter.calls)
	}
}

// Developer 路径同样守住交付标记:显式失败等待对账,不落入业务提交。
func TestDeveloperReconcileRefusesDeliveryMarkerWhileRunRunning(t *testing.T) {
	fixture := newDeveloperRunnerFixture(t, "develop_from_issue")
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start developer execution: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: runner.NewDeliveryMarker(fixture.task.TaskID, fixture.run.RunID),
	}

	_, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err == nil || !strings.Contains(err.Error(), "delivery marker") {
		t.Fatalf("delivery marker on a running run must fail explicitly, got %v", err)
	}
	if runnerResultNeedsConvergence(err) {
		t.Fatalf("delivery marker must wait for reconciliation instead of converging the run: %v", err)
	}
	if fixture.submitter.calls != 0 {
		t.Fatalf("delivery marker must never reach SubmitAgentResult: calls=%d", fixture.submitter.calls)
	}
}
