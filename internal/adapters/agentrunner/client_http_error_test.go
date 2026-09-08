package agentrunner_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/runner"
)

// 池内轮询需要区分「端点应答且明确否定」与传输层失败：非 2xx 响应必须
// 以类型化 HTTPError 浮出，调用方按 StatusCode 判定是否切换端点。
func TestClientReturnsTypedHTTPErrorForNon2xxResponses(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no such execution", http.StatusNotFound)
	}))
	defer server.Close()
	client := agentrunner.NewClient(server.URL, nil)

	_, err := client.Get(context.Background(), "exec-missing")
	var httpErr *runner.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected typed runner.HTTPError, got %#v", err)
	}
	if httpErr.StatusCode != http.StatusNotFound || httpErr.Method != http.MethodGet || httpErr.Path != "/api/v1/executions/exec-missing" {
		t.Fatalf("unexpected HTTPError fields: %#v", httpErr)
	}
	// 消息格式与历史字符串错误逐字一致，消费方错误文案不因类型化漂移。
	want := "agent runner GET /api/v1/executions/exec-missing returned 404: no such execution"
	if err.Error() != want {
		t.Fatalf("error message drifted:\n got %q\nwant %q", err.Error(), want)
	}
}
