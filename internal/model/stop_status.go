package model

import "time"

type StopStatus struct {
	UnknownWritebacks int64                `json:"unknown_writebacks"`
	UnknownMessages   int64                `json:"unknown_messages"`
	StopID            string               `json:"stop_id"`
	Scope             StopScope            `json:"scope"`
	TaskID            string               `json:"task_id"`
	RunID             string               `json:"run_id,omitempty"`
	ExecutionID       string               `json:"execution_id,omitempty"`
	TaskStatus        TaskStatus           `json:"task_status"`
	RunStatus         RunStatus            `json:"run_status,omitempty"`
	ExecutionStatus   AgentExecutionStatus `json:"execution_status,omitempty"`
	DeliveryStatus    StopDeliveryStatus   `json:"delivery_status"`
	ExitConfirmed     bool                 `json:"exit_confirmed"`
	ConfirmationKind  StopConfirmationKind `json:"confirmation_kind,omitempty"`
	RequestedBy       string               `json:"requested_by"`
	RequestedAt       time.Time            `json:"requested_at"`
	ReasonCode        string               `json:"reason_code"`
	Reason            string               `json:"reason,omitempty"`
	ConfirmedAt       *time.Time           `json:"confirmed_at,omitempty"`
}
