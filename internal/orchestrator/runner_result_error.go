package orchestrator

import (
	"errors"

	"sdlc-coordinator/internal/runner"
)

type invalidRunnerResultError struct{ cause error }

func newInvalidRunnerResultError(cause error) error {
	return invalidRunnerResultError{cause: cause}
}

func (e invalidRunnerResultError) Error() string { return e.cause.Error() }

func (e invalidRunnerResultError) Unwrap() error { return e.cause }

type deliveryRunnerResultError struct{ cause error }

func newDeliveryRunnerResultError(cause error) error {
	return deliveryRunnerResultError{cause: cause}
}

func (e deliveryRunnerResultError) Error() string { return e.cause.Error() }

func (e deliveryRunnerResultError) Unwrap() error { return e.cause }

// runnerResultNeedsConvergence reports whether a reconcile error means the
// execution produced a terminal business result that the coordinator cannot
// apply or deliver, so the run must be blocked and the task handed to humans
// instead of being polled again forever.
func runnerResultNeedsConvergence(err error) bool {
	var invalid invalidRunnerResultError
	var delivery deliveryRunnerResultError
	return errors.As(err, &invalid) || errors.As(err, &delivery)
}

// errRunnerDeliveryMarkerPending 表示执行面报告了 MCP 已接受的交付标记,但
// run 尚未终态化:显式失败等待对账,绝不把标记当作业务结果提交。
var errRunnerDeliveryMarkerPending = errors.New("runner reported an MCP-accepted delivery marker while the run is not terminal; waiting for reconciliation")

// refuseDeliveryMarker 拦截 Provider 生成的交付标记:标记不是业务结果,只能
// 在 run 已终态时被吸收(该判断在各 Reconcile 的终态分支完成),绝不能送进
// SubmitAgentResult。
func refuseDeliveryMarker(result *runner.Result) error {
	if result != nil && runner.IsDeliveryMarker(result.StructuredOutput) {
		return errRunnerDeliveryMarkerPending
	}
	return nil
}
