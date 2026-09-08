package orchestrator

import (
	"context"
	"errors"
	"net/http"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
)

// TenantRunnerRouter 是租户 runner 池的派发路由端口：
// 任务 → 租户 → 端点池 → 池内轮询。实现方保证同一租户的池互不交叉
// （隔离红线测试），未指派租户回退默认池。nil 路由 = 单端点
// 直连（既有行为）。
type TenantRunnerRouter interface {
	// AttemptForTask 返回任务派发的首个端点尝试。
	AttemptForTask(ctx context.Context, task *model.Task) (RunnerAttempt, error)
	// ClientForEndpoint 返回钉在指定端点上的客户端（观测亲和）；端点未知
	// 时 ok=false，调用方回退默认客户端。
	ClientForEndpoint(endpoint string) (ReviewerRunnerClient, bool)
}

// RunnerAttempt 是一次端点派发尝试。Create 失败后先在同端点 Get 做崩溃恢复
// （既有语义）；仅当 Get 给出 404 确定性否定（该端点确实没有此执行）才
// Next() 轮换，歧义失败（传输层错误等）终止派发——执行 ID 全局即 run ID，
// 盲目轮换会让同一任务在两个 runner 上重复落执行。
type RunnerAttempt struct {
	Client   ReviewerRunnerClient
	Endpoint string
	next     func() (RunnerAttempt, bool)
}

// NewRunnerAttempt 组装一次端点派发尝试（池实现方使用）；next 可为 nil
// （单端点池，耗尽即止）。
func NewRunnerAttempt(client ReviewerRunnerClient, endpoint string, next func() (RunnerAttempt, bool)) RunnerAttempt {
	return RunnerAttempt{Client: client, Endpoint: endpoint, next: next}
}

// Next 返回池内下一个端点尝试；游标走完返回 ok=false。
func (a RunnerAttempt) Next() (RunnerAttempt, bool) {
	if a.next == nil {
		return RunnerAttempt{}, false
	}
	return a.next()
}

// runnerHTTPStatus 报告 err 是否为携带给定状态码的类型化 HTTP 错误。
func runnerHTTPStatus(err error, statusCode int) bool {
	var httpErr *runner.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == statusCode
}

// runnerNotFound 报告 err 是否为端点应答的 404（确定性否定）。
func runnerNotFound(err error) bool {
	return runnerHTTPStatus(err, http.StatusNotFound)
}

// RunnerCoordinatorOption 定制 reviewer/developer runner 协调器。
type RunnerCoordinatorOption func(*runnerCoordinatorOptions)

type runnerCoordinatorOptions struct {
	router TenantRunnerRouter
}

// WithTenantRunnerRouter 把租户池路由接入协调器核心；nil 保留单端点直连。
func WithTenantRunnerRouter(router TenantRunnerRouter) RunnerCoordinatorOption {
	return func(options *runnerCoordinatorOptions) {
		options.router = router
	}
}

func newRunnerCoordinatorOptions(opts []RunnerCoordinatorOption) *runnerCoordinatorOptions {
	options := &runnerCoordinatorOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}
	return options
}
