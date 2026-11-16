package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Remote execution states recorded as exit-confirmation evidence. A terminal or
// missing remote record is evidence for a human decision, never proof by itself.
const (
	ExecutionExitRemoteTerminal    = "terminal"
	ExecutionExitRemoteNotFound    = "not_found"
	ExecutionExitRemoteUnreachable = "unreachable"
)

// ExecutionExitConfirmation is a durable, immutable human reconciliation that an
// interrupted execution is no longer running. The fingerprint binds it to one
// exact execution row, so any later row change invalidates the evidence.
type ExecutionExitConfirmation struct {
	ID                   uint       `gorm:"primaryKey"`
	ConfirmationID       string     `gorm:"uniqueIndex;not null;size:128"`
	TaskID               string     `gorm:"index;uniqueIndex:idx_execution_exit_request,priority:1;not null;size:128"`
	RunID                string     `gorm:"index;not null;size:128"`
	ExecutionID          string     `gorm:"uniqueIndex:idx_execution_exit_fingerprint,priority:1;not null;size:128"`
	SourceGeneration     uint64     `gorm:"not null"`
	ExecutionFingerprint string     `gorm:"uniqueIndex:idx_execution_exit_fingerprint,priority:2;not null;size:64"`
	ExecutionStatus      string     `gorm:"not null;size:32"`
	ExecutionErrorCode   string     `gorm:"size:64"`
	ExecutionFinishedAt  *time.Time `gorm:"column:execution_finished_at"`
	ExecutionRequestHash string     `gorm:"size:64"`
	RemoteState          string     `gorm:"not null;size:32"`
	RemoteMessage        string     `gorm:"size:1024"`
	RequestID            string     `gorm:"uniqueIndex:idx_execution_exit_request,priority:2;not null;size:128"`
	RequestedBy          string     `gorm:"not null;size:64"`
	Reason               string     `gorm:"not null;size:1000"`
	ConfirmedAt          time.Time  `gorm:"not null"`
	CreatedAt            time.Time
}

// ExecutionExitFingerprint identifies the exact evidence state of one execution
// row. field order is part of the stored value and must not change.
func ExecutionExitFingerprint(execution *AgentExecution) string {
	if execution == nil {
		return ""
	}
	finished := ""
	if execution.FinishedAt != nil {
		finished = execution.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	raw := strings.Join([]string{
		execution.RunID,
		execution.ExecutionID,
		string(execution.Status),
		execution.ErrorCode,
		finished,
		execution.RequestHash,
	}, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
