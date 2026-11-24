package picli

import (
	"encoding/json"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/runner"
)

// streamParser 消费 pi --mode json 输出的 JSONL 事件流:拼接最终助手文本、
// 记录终止失败证据,并在工具完成事件上核验协调器的接受回执。
type streamParser struct {
	emit     runner.EmitFunc
	provider runner.ProviderConfig
	channel  *runner.ResultSubmissionChannel

	taskID       string
	runID        string
	expectedTool string

	finalText       string
	failureMessage  string
	aborted         bool
	acceptedReceipt bool
}

func (p *streamParser) consume(line []byte) error {
	var event map[string]any
	if err := json.Unmarshal(line, &event); err != nil {
		return fmt.Errorf("decode pi event: %w", err)
	}
	switch stringValue(event, "type") {
	case "session":
		payload := map[string]any{"cwd": stringValue(event, "cwd")}
		if id := stringValue(event, "id"); id != "" {
			payload["session_id"] = id
		}
		p.emitEvent("pi_session", payload)
	case "message_end":
		p.consumeMessageEnd(event)
	case "tool_execution_start":
		p.emitEvent("pi_progress", map[string]any{
			"item_type": "tool_call", "tool": stringValue(event, "toolName"), "status": "in_progress",
		})
	case "tool_execution_end":
		isError, _ := event["isError"].(bool)
		p.emitEvent("pi_progress", map[string]any{
			"item_type": "tool_call", "tool": stringValue(event, "toolName"), "status": "completed", "is_error": isError,
		})
		p.observeAcceptanceReceipt(event, isError)
	case "auto_retry_start":
		p.emitEvent("pi_retry", map[string]any{
			"attempt": event["attempt"], "max_attempts": event["maxAttempts"], "delay_ms": event["delayMs"],
		})
	case "auto_retry_end":
		if success, _ := event["success"].(bool); !success {
			p.recordFailure(stringValue(event, "finalError"))
		}
	case "compaction_start", "compaction_end":
		p.emitEvent("pi_compaction", selectFields(event, "reason", "aborted"))
	case "agent_settled":
		if aborted, _ := event["aborted"].(bool); aborted {
			p.aborted = true
		}
	}
	return nil
}

// consumeMessageEnd 只采信 assistant 消息:该系统消息与用户消息同型,把
// 它们当终稿会把提示词或系统提示词误判成业务结果。
func (p *streamParser) consumeMessageEnd(event map[string]any) {
	message, _ := event["message"].(map[string]any)
	if stringValue(message, "role") != "assistant" {
		return
	}
	texts := make([]string, 0, 1)
	for _, block := range contentBlocks(message) {
		if stringValue(block, "type") != "text" {
			continue
		}
		if text, _ := block["text"].(string); text != "" {
			texts = append(texts, text)
		}
	}
	// 终稿以最后一条 assistant 消息为准:更早轮次的文本只描述中间进度。
	p.finalText = strings.Join(texts, "\n")
	payload := map[string]any{"stop_reason": stringValue(message, "stopReason")}
	if usage, ok := message["usage"].(map[string]any); ok {
		payload["usage"] = selectFields(usage, "input", "output", "cacheRead", "cacheWrite", "totalTokens", "cost")
	}
	p.emitEvent("pi_usage", payload)
	if stringValue(message, "stopReason") == "error" {
		p.recordFailure(stringValue(message, "errorMessage"))
		return
	}
	// 该轮成功即说明执行仍在推进,此前轮次的失败已被 pi 自身处理掉。
	p.failureMessage = ""
}

func (p *streamParser) recordFailure(message string) {
	if message == "" {
		return
	}
	p.failureMessage = p.redact(message)
}

// observeAcceptanceReceipt 只在结构化的工具完成事件上采信接受回执:工具
// 无错误、details 指向协调器与角色工具、回执文本的 status/task_id/run_id/
// run_status 全部对齐才置位。
func (p *streamParser) observeAcceptanceReceipt(event map[string]any, isError bool) {
	if p.acceptedReceipt || p.channel == nil || p.expectedTool == "" || isError {
		return
	}
	result, ok := event["result"].(map[string]any)
	if !ok {
		return
	}
	details, _ := result["details"].(map[string]any)
	if stringValue(details, "server") != mcpServerName || stringValue(details, "tool") != p.expectedTool {
		return
	}
	if _, ok := runner.ParseMCPAcceptedReceipt(mcpResultText(result), p.taskID, p.runID); !ok {
		return
	}
	p.acceptedReceipt = true
}

// mcpResultText 拼接工具结果里的文本块,与 codex Provider 的回执解析口径一致。
func mcpResultText(result map[string]any) string {
	content, _ := result["content"].([]any)
	texts := make([]string, 0, len(content))
	for _, block := range content {
		blockMap, _ := block.(map[string]any)
		if text, _ := blockMap["text"].(string); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

// deliveryMarker 只在本 run 的 MCP 结果已被接受时返回交付标记。
func (p *streamParser) deliveryMarker() json.RawMessage {
	if !p.acceptedReceipt {
		return nil
	}
	return runner.NewDeliveryMarker(p.taskID, p.runID)
}

func (p *streamParser) emitEvent(eventType string, payload map[string]any) {
	if p.emit != nil {
		p.emit(eventType, payload)
	}
}

func (p *streamParser) redact(message string) string {
	return redactProviderError(message, p.provider, p.channel)
}

func (p *streamParser) emitFailure(code, message string) {
	payload := map[string]any{"error_code": strings.TrimSpace(code)}
	if text := strings.TrimSpace(message); text != "" {
		payload["message"] = p.redact(text)
	}
	p.emitEvent("pi_error", payload)
}

func contentBlocks(message map[string]any) []map[string]any {
	blocks, _ := message["content"].([]any)
	result := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		if blockMap, ok := block.(map[string]any); ok {
			result = append(result, blockMap)
		}
	}
	return result
}

func selectFields(source map[string]any, names ...string) map[string]any {
	result := make(map[string]any, len(names))
	for _, name := range names {
		if value, exists := source[name]; exists {
			result[name] = value
		}
	}
	return result
}

func stringValue(source map[string]any, name string) string {
	value, _ := source[name].(string)
	return strings.TrimSpace(value)
}
