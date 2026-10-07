package model

import "time"

// DeveloperDeliveryCheckpoint 是 Developer 交付的 Coordinator 私有检查点。
// prepare 成功后钉住受信基线(每 run 一行,run 唯一),交付提交创建后、push
// 前以条件更新写入提交 SHA。租户库对 Agent 不可写,检查点是"提交由
// Coordinator 创建"的唯一归属凭据;共享工作区里可伪造的尾注、身份与消息
// 形态都不构成证明。仅存交付元数据,不存 token/密钥。
type DeveloperDeliveryCheckpoint struct {
	ID       uint   `gorm:"primaryKey"`
	TaskID   string `gorm:"index;not null;size:128"`
	RunID    string `gorm:"uniqueIndex;not null;size:128"`
	TaskType string `gorm:"column:task_type;not null"`
	// RepositoryURL/DeliveryBranch 与受信任务元数据绑定,防止检查点被复用
	// 到其他仓或分支。
	RepositoryURL  string `gorm:"column:repository_url;type:text;not null"`
	DeliveryBranch string `gorm:"column:delivery_branch;not null"`
	// BaselineSHA 是 prepare 时的初始 HEAD:固定基线,新建交付提交必须是
	// 它的直接子提交,重试推送也以它为远端漂移判据。
	BaselineSHA string `gorm:"column:baseline_sha;not null"`
	// CommitSHA 为空=基线已钉、交付提交未创建;只允许从空翻转一次,翻转后
	// 本 run 至多一笔由流程认可的交付提交。
	CommitSHA string `gorm:"column:commit_sha;index;size:64"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
