package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/resulttoken"
	"sdlc-coordinator/internal/store/repository"

	"github.com/gin-gonic/gin"
)

const mcpProtocolFallbackVersion = "2025-06-18"

// MCPHandler serves the agent result submission channel: a stateless
// Streamable-HTTP MCP endpoint at /mcp. Requests authenticate with the
// per-run bearer token issued at dispatch; correction attempts are counted
// per run via persisted rejection rows.
type MCPHandler struct {
	channel     *orchestrator.ResultChannelCoordinator
	secret      []byte
	tasks       orchestrator.TaskRepository
	runs        orchestrator.RunRepository
	rejections  *repository.ResultRejectionRepository
	maxAttempts int
	now         func() time.Time
}

func NewMCPHandler(
	channel *orchestrator.ResultChannelCoordinator,
	secret []byte,
	tasks orchestrator.TaskRepository,
	runs orchestrator.RunRepository,
	rejections *repository.ResultRejectionRepository,
	maxAttempts int,
) *MCPHandler {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	return &MCPHandler{
		channel: channel, secret: secret, tasks: tasks, runs: runs,
		rejections: rejections, maxAttempts: maxAttempts, now: time.Now,
	}
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Name            string          `json:"name"`
		Arguments       json.RawMessage `json:"arguments"`
	} `json:"params"`
}

// mcpToolPayload is the correction-protocol text carried inside isError tool
// results: verbatim contract problems with parameter paths, the authoritative
// per-run attempt accounting, and an explicit next step.
type mcpToolPayload struct {
	Status            string                           `json:"status"`
	Attempt           int64                            `json:"attempt,omitempty"`
	MaxAttempts       int                              `json:"max_attempts,omitempty"`
	AttemptsRemaining int64                            `json:"attempts_remaining,omitempty"`
	TaskID            string                           `json:"task_id,omitempty"`
	RunID             string                           `json:"run_id,omitempty"`
	TaskStatus        model.TaskStatus                 `json:"task_status,omitempty"`
	RunStatus         model.RunStatus                  `json:"run_status,omitempty"`
	Normalized        bool                             `json:"normalized,omitempty"`
	Problems          []orchestrator.ValidationProblem `json:"problems,omitempty"`
	NextStep          string                           `json:"next_step,omitempty"`
}

const (
	mcpStatusAccepted     = "accepted"
	mcpStatusRejected     = "rejected"
	mcpStatusRejectedFina = "rejected_final"
	mcpStatusAlreadyFinal = "already_final"
	mcpStatusProcessing   = "processing"
)

func (h *MCPHandler) Handle(c *gin.Context) {
	if h == nil || h.channel == nil || len(h.secret) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mcp handler is not configured"})
		return
	}
	if c.Request.Method == http.MethodGet {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "SSE streaming is not supported; POST JSON-RPC only"})
		return
	}
	claims, status := h.authenticate(c)
	if status != 0 {
		c.JSON(status, gin.H{"error": "unauthorized"})
		return
	}

	var request mcpRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, mcpErrorResponse(nil, -32700, "Parse error", err.Error()))
		return
	}
	if request.JSONRPC != "2.0" {
		c.JSON(http.StatusOK, mcpErrorResponse(request.ID, -32600, "Invalid Request", "jsonrpc must be 2.0"))
		return
	}

	switch request.Method {
	case "initialize":
		// 无状态会话：回显客户端协议版本，签发一个不校验后续一致性的
		// 会话头（codex/claude 客户端都兼容）。
		version := strings.TrimSpace(request.Params.ProtocolVersion)
		if version == "" {
			version = mcpProtocolFallbackVersion
		}
		c.Header("Mcp-Session-Id", model.NewID("mcp-session"))
		c.JSON(http.StatusOK, mcpResultResponse(request.ID, gin.H{
			"protocolVersion": version,
			"capabilities":    gin.H{"tools": gin.H{}},
			"serverInfo":      gin.H{"name": "sdlc-coordinator", "title": "Coordinator Result Channel", "version": "1.0.0"},
		}))
	case "tools/list":
		c.JSON(http.StatusOK, mcpResultResponse(request.ID, gin.H{"tools": h.toolsForRole(claims.Role)}))
	case "tools/call":
		if response := h.callTool(c, claims, request); response != nil {
			c.JSON(http.StatusOK, response)
		}
	default:
		c.JSON(http.StatusOK, mcpErrorResponse(request.ID, -32601, "Method not found", request.Method))
	}
}

// CloseSession answers the DELETE a session teardown sends (codex rmcp sends
// one on close). Stateless server: nothing to invalidate.
func (h *MCPHandler) CloseSession(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (h *MCPHandler) authenticate(c *gin.Context) (resulttoken.Claims, int) {
	header := c.GetHeader("Authorization")
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		return resulttoken.Claims{}, http.StatusUnauthorized
	}
	claims, err := resulttoken.Verify(h.secret, strings.TrimSpace(token), h.now())
	if err != nil {
		return resulttoken.Claims{}, http.StatusUnauthorized
	}
	return claims, 0
}

func (h *MCPHandler) toolsForRole(role string) []gin.H {
	toolName, schema, ok := contractForWorkerRole(role)
	if !ok {
		return []gin.H{}
	}
	return []gin.H{{
		"name":        toolName,
		"title":       "Submit " + workerRoleLabel(role) + " Result",
		"description": "Submit the structured result for the current run. Validation errors come back as problems; fix the listed parameters and call again.",
		"inputSchema": schema,
	}}
}

// contractForWorkerRole exposes the tool identity and published schema of a
// run's role. The developer role publishes the agent-shaped schema (no
// delivery evidence: the coordinator finalizes git delivery itself).
func contractForWorkerRole(role string) (string, map[string]any, bool) {
	switch role {
	case "DeveloperWorker":
		agent := results.NewDeveloperAgentContract()
		return results.SubmitDeveloperResultTool, agent.InputSchema(), true
	case "ReviewerWorker":
		contract := results.NewReviewerContract()
		return contract.ToolName(), contract.InputSchema(), true
	case "TesterWorker":
		contract := results.NewTesterContract()
		return contract.ToolName(), contract.InputSchema(), true
	default:
		return "", nil, false
	}
}

func workerRoleLabel(role string) string {
	switch role {
	case "DeveloperWorker":
		return "Developer"
	case "ReviewerWorker":
		return "Reviewer"
	case "TesterWorker":
		return "Tester"
	default:
		return role
	}
}

func (h *MCPHandler) callTool(c *gin.Context, claims resulttoken.Claims, request mcpRequest) gin.H {
	toolName, _, ok := contractForWorkerRole(claims.Role)
	if !ok || toolName != request.Params.Name {
		// 角色越界（如 developer token 调 reviewer 工具）：直接 403，不计数。
		c.AbortWithStatusJSON(http.StatusForbidden, mcpErrorResponse(request.ID, -32602, "Invalid params", "tool is not available for this run's role"))
		return nil
	}
	if len(request.Params.Arguments) == 0 {
		return toolResultResponse(request.ID, true, mcpToolPayload{
			Status: mcpStatusRejected, MaxAttempts: h.maxAttempts,
			Problems: []orchestrator.ValidationProblem{{Problem: "arguments is required"}},
			NextStep: "提交参数不能为空；按工具入参 schema 重新调用。",
		})
	}

	run, err := h.runs.GetByRunID(c.Request.Context(), claims.RunID)
	if err != nil {
		// 签名有效但 run 不存在/已清理：按越界处理（fail-closed，不计数）。
		c.AbortWithStatusJSON(http.StatusForbidden, mcpErrorResponse(request.ID, -32602, "Invalid params", "forbidden"))
		return nil
	}
	if run.TaskID != claims.TaskID || run.WorkerRole != claims.Role {
		c.AbortWithStatusJSON(http.StatusForbidden, mcpErrorResponse(request.ID, -32602, "Invalid params", "forbidden"))
		return nil
	}
	if run.Status != model.RunStatusRunning {
		// 首提即终态：后续任何提交（同哈希重放或新载荷）只收到已终态提示。
		return toolResultResponse(request.ID, true, mcpToolPayload{
			Status: mcpStatusAlreadyFinal, TaskID: run.TaskID, RunID: run.RunID, RunStatus: run.Status,
			NextStep: "本 run 已按先前提交终态化；不要再调用本工具，直接停止。",
		})
	}
	rejected, err := h.rejections.CountByRun(c.Request.Context(), run.RunID)
	if err != nil {
		return mcpErrorResponse(request.ID, -32603, "Internal error", err.Error())
	}
	if rejected >= int64(h.maxAttempts) {
		return h.handoffResponse(request.ID, run, rejected)
	}

	task, err := h.tasks.GetByTaskID(c.Request.Context(), run.TaskID)
	if err != nil {
		return mcpErrorResponse(request.ID, -32602, "Invalid params", "task for this token does not exist")
	}

	outcome, err := h.channel.SubmitAgentResult(c.Request.Context(), task, run, request.Params.Name, request.Params.Arguments)
	if err != nil {
		return mcpErrorResponse(request.ID, -32603, "Internal error", err.Error())
	}
	switch outcome.Decision {
	case orchestrator.ChannelAccepted:
		payload := mcpToolPayload{
			Status: mcpStatusAccepted, TaskID: task.TaskID, RunID: run.RunID,
			TaskStatus: outcome.Response.Task.Status, RunStatus: outcome.Response.Run.Status,
			Normalized: outcome.Normalized, MaxAttempts: h.maxAttempts,
			NextStep: "结果已接受，run 已终态化。立即停止，不要再调用本工具，也不要再输出结果 JSON。",
		}
		return toolResultResponse(request.ID, false, payload)
	case orchestrator.ChannelRejected:
		attempt := rejected + 1
		if err := h.rejections.Record(c.Request.Context(), &model.ResultSubmissionRejection{
			RunID: run.RunID, TaskID: task.TaskID, ToolName: request.Params.Name, Role: run.WorkerRole,
			ProblemsJSON: encodeProblems(outcome.Problems),
		}); err != nil {
			return mcpErrorResponse(request.ID, -32603, "Internal error", err.Error())
		}
		if attempt >= int64(h.maxAttempts) {
			return h.handoffWithProblems(request.ID, run, attempt, outcome.Problems)
		}
		return toolResultResponse(request.ID, true, mcpToolPayload{
			Status: mcpStatusRejected, Attempt: attempt, MaxAttempts: h.maxAttempts,
			AttemptsRemaining: int64(h.maxAttempts) - attempt,
			TaskID:            task.TaskID, RunID: run.RunID,
			Problems: outcome.Problems, Normalized: outcome.Normalized,
			NextStep: "按 problems 修正参数后重新调用本工具；错误不修正无法通过校验。",
		})
	default: // ChannelFailed：传输/内部类错误——不计校验次数，指引文本通道。
		switch {
		case errors.Is(outcome.Err, orchestrator.ErrResultConflict):
			return toolResultResponse(request.ID, true, mcpToolPayload{
				Status: mcpStatusAlreadyFinal, TaskID: task.TaskID, RunID: run.RunID,
				NextStep: "本 run 已按另一份提交终态化；不要再调用本工具，直接停止。",
			})
		case errors.Is(outcome.Err, orchestrator.ErrResultProcessing):
			return toolResultResponse(request.ID, true, mcpToolPayload{
				Status: mcpStatusProcessing, TaskID: task.TaskID, RunID: run.RunID,
				NextStep: "先前提交正在收尾，稍候片刻后重试一次；此错误不消耗矫正次数。",
			})
		default:
			// 通道内部故障：指示文本通道（不计次数）。
			return toolResultResponse(request.ID, true, mcpToolPayload{
				Status: mcpStatusRejected, MaxAttempts: h.maxAttempts, TaskID: task.TaskID, RunID: run.RunID,
				Problems: []orchestrator.ValidationProblem{{Problem: "submission channel is temporarily unavailable"}},
				NextStep: "提交通道暂时不可用（此错误不消耗矫正次数）。按任务提示词中的结果契约把最终消息写成 JSON 文本，然后停止。",
			})
		}
	}
}

// handoffResponse is the post-exhaustion answer: no new counting, no
// re-validation, just the standing instruction to use the text channel.
func (h *MCPHandler) handoffResponse(id json.RawMessage, run *model.Run, rejected int64) gin.H {
	return h.handoffWithProblems(id, run, rejected, nil)
}

func (h *MCPHandler) handoffWithProblems(id json.RawMessage, run *model.Run, attempt int64, problems []orchestrator.ValidationProblem) gin.H {
	remaining := int64(0)
	return toolResultResponse(id, true, mcpToolPayload{
		Status: mcpStatusRejectedFina, Attempt: attempt, MaxAttempts: h.maxAttempts,
		AttemptsRemaining: remaining, TaskID: run.TaskID, RunID: run.RunID,
		Problems: problems,
		NextStep: "矫正次数已用尽。不要再调用本工具：按任务提示词中的结果契约，把最终消息写成 JSON 文本，然后停止。",
	})
}

func encodeProblems(problems []orchestrator.ValidationProblem) string {
	if len(problems) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(problems)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func mcpResultResponse(id json.RawMessage, result any) gin.H {
	return gin.H{"jsonrpc": "2.0", "id": id, "result": result}
}

func mcpErrorResponse(id json.RawMessage, code int, message string, data any) gin.H {
	return gin.H{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   gin.H{"code": code, "message": message, "data": data},
	}
}

func toolResultResponse(id json.RawMessage, isError bool, payload mcpToolPayload) gin.H {
	text, err := json.Marshal(payload)
	if err != nil {
		text = []byte(`{"status":"rejected","problems":[{"problem":"response encoding failed"}]}`)
	}
	return mcpResultResponse(id, gin.H{
		"content": []gin.H{{"type": "text", "text": string(text)}},
		"isError": isError,
	})
}
