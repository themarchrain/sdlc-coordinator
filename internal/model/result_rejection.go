package model

import "time"

// ResultSubmissionRejection is one rejected MCP submission attempt. Rejected
// attempts never create a ResultSubmission row, so this table is both the
// audit trail and the authoritative per-run correction counter (COUNT by
// run_id); bounded by the correction attempt cap.
type ResultSubmissionRejection struct {
	ID           uint      `gorm:"primaryKey"`
	RunID        string    `gorm:"index;not null;size:128"`
	TaskID       string    `gorm:"index;not null;size:128"`
	ToolName     string    `gorm:"not null"`
	Role         string    `gorm:"not null"`
	ProblemsJSON string    `gorm:"type:text;not null"`
	CreatedAt    time.Time `gorm:"index"`
}
