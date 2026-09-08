package runner

import "fmt"

// HTTPError 是 agent runner 客户端的类型化 HTTP 状态错误：非 2xx 响应统一
// 包装为本类型，消息格式与历史字符串错误逐字一致。池内轮询据此区分
// 「端点应答且明确否定」（如 404，可安全切换下一端点）与传输层失败（连接
// 拒绝/超时，歧义状态，禁止切换以免同任务在两个 runner 上重复落执行）。
type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("agent runner %s %s returned %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}
