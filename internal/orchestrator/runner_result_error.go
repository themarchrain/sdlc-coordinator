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

// refuseDeliveryMarker 拦截 Provider 生成的交付标记:标记只能在 run 已终态时
// 被吸收(该判断在各 Reconcile 的终态分支完成)。run 未终态却出现标记说明
// 接受声明与协调器状态对不上(接受会同步终态化 run),按无效结果显式收敛:
// 阻断 run、转人工,绝不送进业务结果通道,也不留下无限对账的空转 run。
func refuseDeliveryMarker(result *runner.Result) error {
	if result != nil && runner.IsDeliveryMarker(result.StructuredOutput) {
		return newInvalidRunnerResultError(errors.New("runner result claims an MCP-accepted delivery while the run is not terminal"))
	}
	return nil
}
