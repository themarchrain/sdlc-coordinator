package model

import "time"

// MergeGateAudit 记录自动合并门禁的每次裁决:放行依据(消费了哪些证据、
// 各自取值)、执行结果、回落人工的逐项原因。每次自动合并尝试都留痕,
// 回落也是;每验证任务至多尝试一次,由 task_id 唯一约束承担防重。
type MergeGateAudit struct {
	ID              uint   `gorm:"primaryKey"`
	AuditID         string `gorm:"uniqueIndex;not null"`
	TaskID          string `gorm:"index;not null;uniqueIndex:idx_merge_gate_task"`
	RunID           string `gorm:"index"`
	RepositoryID    string `gorm:"index;not null"`
	ProjectPath     string
	MergeRequestIID int
	HeadSHA         string
	// Decision 取值 merged / fallback:merged=平台接受合并,fallback=任一
	// 环缺失或平台拒绝而回落人工,原因逐项写在 ReasonsJSON。
	Decision        string `gorm:"index"`
	ReasonsJSON     string `gorm:"type:text"`
	MergedCommitSHA string
	ActorKind       string
	CreatedAt       time.Time
}
