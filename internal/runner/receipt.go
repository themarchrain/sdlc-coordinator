package runner

import (
	"encoding/json"
	"strings"

	"sdlc-coordinator/internal/model"
)

// deliveryMarkerField 是 Provider 生成的交付标记的外层 JSON 字段名。标记
// 不是 Agent 业务结果:它只表明本次 run 的业务结果已通过协调器 MCP 主通道
// 被接受,而 CLI 无法再提供可归一化的最终 JSON。协调器只可在 run 已终态时
// 吸收该标记,绝不能把它当作业务 JSON 提交。
const deliveryMarkerField = "delivery_marker"

// deliveryMarkerReasonMCPAccepted 是交付标记唯一的产生原因。
const deliveryMarkerReasonMCPAccepted = "mcp_accepted"

type deliveryMarkerPayload struct {
	TaskID string `json:"task_id"`
	RunID  string `json:"run_id"`
	Reason string `json:"reason"`
}

// NewDeliveryMarker 生成携带本次执行身份的交付标记。
func NewDeliveryMarker(taskID, runID string) json.RawMessage {
	encoded, err := json.Marshal(map[string]deliveryMarkerPayload{
		deliveryMarkerField: {TaskID: taskID, RunID: runID, Reason: deliveryMarkerReasonMCPAccepted},
	})
	if err != nil {
		return nil
	}
	return encoded
}

// IsDeliveryMarker 判定结构化结果是否为 Provider 生成的交付标记。
func IsDeliveryMarker(raw []byte) bool {
	marker, ok := decodeDeliveryMarker(raw)
	return ok && marker.TaskID != "" && marker.RunID != "" && marker.Reason == deliveryMarkerReasonMCPAccepted
}

func decodeDeliveryMarker(raw []byte) (deliveryMarkerPayload, bool) {
	if len(raw) == 0 {
		return deliveryMarkerPayload{}, false
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return deliveryMarkerPayload{}, false
	}
	encoded, ok := envelope[deliveryMarkerField]
	if !ok {
		return deliveryMarkerPayload{}, false
	}
	var marker deliveryMarkerPayload
	if err := json.Unmarshal(encoded, &marker); err != nil {
		return deliveryMarkerPayload{}, false
	}
	return marker, true
}

// MCPAcceptedReceipt 是协调器 MCP 成功回执中可采信的最小事实集合。
type MCPAcceptedReceipt struct {
	TaskID    string
	RunID     string
	RunStatus model.RunStatus
}

// ParseMCPAcceptedReceipt 严格校验工具结果文本:必须是单个 JSON 对象,
// status 为 accepted,task_id 与 run_id 和当前执行一致,且 run_status 已
// 终态化。任何一项不符都返回 false,调用方必须回退到既有失败路径。
func ParseMCPAcceptedReceipt(text, taskID, runID string) (MCPAcceptedReceipt, bool) {
	if taskID == "" || runID == "" {
		return MCPAcceptedReceipt{}, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return MCPAcceptedReceipt{}, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &fields); err != nil {
		return MCPAcceptedReceipt{}, false
	}
	if rawJSONString(fields["status"]) != "accepted" {
		return MCPAcceptedReceipt{}, false
	}
	receipt := MCPAcceptedReceipt{
		TaskID:    rawJSONString(fields["task_id"]),
		RunID:     rawJSONString(fields["run_id"]),
		RunStatus: model.RunStatus(rawJSONString(fields["run_status"])),
	}
	if receipt.TaskID != taskID || receipt.RunID != runID || !terminalRunStatus(receipt.RunStatus) {
		return MCPAcceptedReceipt{}, false
	}
	return receipt, true
}

func rawJSONString(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func terminalRunStatus(status model.RunStatus) bool {
	switch status {
	case model.RunStatusSucceeded, model.RunStatusFailed, model.RunStatusBlocked:
		return true
	default:
		return false
	}
}
