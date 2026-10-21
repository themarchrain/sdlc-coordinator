package model

import "time"

type ActionKind string

const (
	ActionExecutionCreate ActionKind = "execution_create"
	ActionGitCommit       ActionKind = "git_commit"
	ActionGitPush         ActionKind = "git_push"
	ActionWriteback       ActionKind = "writeback"
	ActionIMMessage       ActionKind = "im_message"
	ActionAutoMerge       ActionKind = "auto_merge"
)

type ActionAdmission struct {
	ID              uint       `gorm:"primaryKey"`
	AdmissionID     string     `gorm:"uniqueIndex;not null;size:128"`
	TaskID          string     `gorm:"index;not null;size:128"`
	RunID           string     `gorm:"index;size:128"`
	Kind            ActionKind `gorm:"uniqueIndex:idx_action_admission_attempt,priority:1;not null;size:32"`
	ActionKey       string     `gorm:"uniqueIndex:idx_action_admission_attempt,priority:2;not null;size:255"`
	Attempt         int        `gorm:"uniqueIndex:idx_action_admission_attempt,priority:3;not null"`
	LeaseOwner      string     `gorm:"not null;size:128"`
	LeaseGeneration uint64     `gorm:"not null"`
	AdmittedAt      time.Time  `gorm:"not null"`
}
