package model

import (
	"time"

	"gorm.io/gorm"
)

type TaskStatus string

const (
	TaskStatusPending         TaskStatus = "pending"
	TaskStatusClaimed         TaskStatus = "claimed"
	TaskStatusRunning         TaskStatus = "running"
	TaskStatusFinalizing      TaskStatus = "finalizing"
	TaskStatusSucceeded       TaskStatus = "succeeded"
	TaskStatusWaitingUpstream TaskStatus = "waiting_upstream"
	TaskStatusWaitingMerge    TaskStatus = "waiting_merge"
	TaskStatusRetryPending    TaskStatus = "retry_pending"
	TaskStatusBlocked         TaskStatus = "blocked"
	TaskStatusHumanRequired   TaskStatus = "human_required"
)

type Task struct {
	ID                 uint       `gorm:"primaryKey"`
	TaskID             string     `gorm:"uniqueIndex;not null;size:128"`
	DedupeKey          string     `gorm:"uniqueIndex;not null;size:255"`
	RepositoryID       *string    `gorm:"column:repository_id;index;size:128"`
	Type               string     `gorm:"index;not null;size:64"`
	Role               string     `gorm:"index;not null;size:32"`
	SourceRepo         string     `gorm:"index;size:255"`
	TargetRepo         string     `gorm:"index;size:255"`
	Objective          string     `gorm:"type:text"`
	AcceptanceCriteria string     `gorm:"type:text"`
	Status             TaskStatus `gorm:"index;not null;size:32"`
	LeaseUntil         *time.Time `gorm:"index"`
	AttemptCount       int        `gorm:"not null;default:0"`
	CurrentRunID       *string    `gorm:"index;size:128"`
	ContextJSON        string     `gorm:"type:text"`
	CallbackURL        string     `gorm:"type:text"`
	// DeliveryBranch 带索引:验证处方按它反查产生源分支的开发任务,是
	// 派发热路径上的等值查询。
	DeliveryBranch string `gorm:"size:255;index"`
	LastError      string `gorm:"type:text"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DeveloperResumeRequest struct {
	DedupeKey     string
	NoteID        int
	ResumeContext string
	AssigneeIDs   []int
}

func (t *Task) BeforeCreate(tx *gorm.DB) error {
	if t.TaskID == "" {
		t.TaskID = NewID("task")
	}
	if t.Status == "" {
		t.Status = TaskStatusPending
	}
	return nil
}
