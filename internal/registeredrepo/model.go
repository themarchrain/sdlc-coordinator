package registeredrepo

import (
	"strings"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type RepositoryStatus string

const (
	StatusConfiguring RepositoryStatus = "configuring"
	StatusEnabled     RepositoryStatus = "enabled"
	StatusDisabled    RepositoryStatus = "disabled"
)

func ValidStatus(status RepositoryStatus) bool {
	switch status {
	case StatusConfiguring, StatusEnabled, StatusDisabled:
		return true
	default:
		return false
	}
}

type RegisteredRepository struct {
	ID           uint   `gorm:"primaryKey"`
	RepositoryID string `gorm:"column:repository_id;uniqueIndex;not null"`
	DisplayName  string `gorm:"column:display_name;not null"`
	// Provider/BaseURL 是仓的平台身份锚点:消费链(验签/写回/探针/人员/需求)
	// 一律按仓行解析。全局 codehost 配置只作为注册表单默认值与空列回填来源。
	Provider string `gorm:"column:provider;index;not null;default:''"`
	BaseURL  string `gorm:"column:base_url;type:text"`
	// CodeHostProjectID 是平台侧数字对象 id(gitlab/gitea 的 project id、github
	// repository id),身份锚点之一;换平台后旧值不可比,由服务层清空重探。
	CodeHostProjectID string `gorm:"column:codehost_project_id"`
	// PathWithNamespace 的唯一性由 provider 作用域索引承担
	// (EnsureIdentityIndexes),跨平台同 path 是合法共存。
	PathWithNamespace   string `gorm:"column:path_with_namespace;not null"`
	Token               string `gorm:"column:token;type:text"`
	WebhookSecret       string `gorm:"column:webhook_secret;type:text"`
	TokenScopeAttested  bool   `gorm:"column:token_scope_attested;not null;default:false"`
	DefaultBranch       string `gorm:"column:default_branch;type:text"`
	CodeHostIssueLabels string `gorm:"column:codehost_issue_labels;type:text"`
	ReviewerMentions    string `gorm:"column:reviewer_mentions;type:text"`
	// VerifyCommand 是仓库级声明的验证命令下限,验证任务派发时并进处方;
	// 空=仓库未声明下限,验证只复放开发者记录的命令。
	VerifyCommand string `gorm:"column:verify_command;type:text"`
	// GitAuthorName/GitAuthorEmail 是 Developer 交付提交的作者与提交者身份,
	// 成对配置、仅停用态可改;空=未配置,运行时降级使用内置身份。它们进入
	// 公开 Git 提交元数据,不是凭据,与 Token 严格分域。
	GitAuthorName          string           `gorm:"column:git_author_name;type:text"`
	GitAuthorEmail         string           `gorm:"column:git_author_email;type:text"`
	Status                 RepositoryStatus `gorm:"column:status;index;not null"`
	LastConfigurationError string           `gorm:"column:last_configuration_error;type:text"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (repo *RegisteredRepository) BeforeCreate(tx *gorm.DB) error {
	if strings.TrimSpace(repo.RepositoryID) == "" {
		repo.RepositoryID = model.NewID("repo")
	}
	if repo.Status == "" {
		repo.Status = StatusConfiguring
	}
	return nil
}

// CloneURL 由仓行身份字段派生 http(s) clone 地址。注册仓行是 workspace
// clone/push 的唯一权威源,事件载荷中的 repository_url 仅作展示;
// base_url 或 path 缺失时返回空串,由消费方显式报错。
func (repo *RegisteredRepository) CloneURL() string {
	base := strings.TrimRight(strings.TrimSpace(repo.BaseURL), "/")
	repoPath := strings.TrimSpace(repo.PathWithNamespace)
	if base == "" || repoPath == "" {
		return ""
	}
	return base + "/" + repoPath + ".git"
}
