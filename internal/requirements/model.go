package requirements

import (
	"encoding/json"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusPublished Status = "published"
	StatusDone      Status = "done"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// DeveloperAgentLabel 是三平台 normalizer 的 issue→任务门禁标签,
// 需求建 issue 时必须自动追加,否则 webhook 回环不会产生开发任务。
const DeveloperAgentLabel = "developer-agent"

func ValidStatus(status Status) bool {
	switch status {
	case StatusQueued, StatusPublished, StatusDone, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

func ValidPriority(priority string) bool {
	switch priority {
	case "", "high", "medium", "low":
		return true
	default:
		return false
	}
}

type Requirement struct {
	ID            uint      `gorm:"primaryKey"`
	RequirementID string    `gorm:"column:requirement_id;uniqueIndex;not null;size:128"`
	Title         string    `gorm:"column:title;not null"`
	Description   string    `gorm:"column:description;type:text"`
	Status        Status    `gorm:"column:status;index;not null;size:32"`
	RepositoryID  string    `gorm:"column:repository_id;index;not null;size:128"`
	Labels        string    `gorm:"column:labels;type:text"`
	Priority      string    `gorm:"column:priority;index;not null;default:'';size:16"`
	IssueIID      int       `gorm:"column:issue_iid;index;not null;default:0"`
	IssueURL      string    `gorm:"column:issue_url;type:text"`
	LastError     string    `gorm:"column:last_error;type:text"`
	PublishedAt   *time.Time
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (item *Requirement) BeforeCreate(tx *gorm.DB) error {
	if strings.TrimSpace(item.RequirementID) == "" {
		item.RequirementID = model.NewID("req")
	}
	if item.Status == "" {
		item.Status = StatusQueued
	}
	return nil
}

// SourceKey 是需求的稳定业务身份,同时用作 issue 描述内嵌签名与 reconcile 回查标记。
func (item *Requirement) SourceKey() string {
	return "req:" + item.RequirementID
}

func (item *Requirement) LabelList() []string {
	return DecodeLabels(item.Labels)
}

func EncodeLabels(values []string) string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			trimmed = append(trimmed, value)
		}
	}
	if len(trimmed) == 0 {
		return ""
	}
	encoded, err := json.Marshal(trimmed)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func DecodeLabels(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func MergeDeveloperAgentLabel(labels []string) []string {
	for _, label := range labels {
		if strings.EqualFold(strings.TrimSpace(label), DeveloperAgentLabel) {
			return labels
		}
	}
	merged := make([]string, 0, len(labels)+1)
	merged = append(merged, labels...)
	return append(merged, DeveloperAgentLabel)
}

// RenderDescription 生成建单描述:原文 + 明文来源行 + 空行 + 不可见签名注释。
func RenderDescription(description, sourceKey string) string {
	body := strings.TrimRight(description, "\r\n \t")
	var builder strings.Builder
	if body != "" {
		builder.WriteString(body)
		builder.WriteString("\n\n")
	}
	builder.WriteString("> 来自协调器需求 ")
	builder.WriteString(sourceKey)
	builder.WriteString("\n\n<!-- req-source-key: ")
	builder.WriteString(sourceKey)
	builder.WriteString(" -->")
	return builder.String()
}
