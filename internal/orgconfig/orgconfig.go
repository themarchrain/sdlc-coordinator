// Package orgconfig 承载组织段运行配置（代码仓库默认值、接续开发白名单、
// IM 通知、固定提醒人）：每组织一份，落在该组织的租户库，保存即热生效。
package orgconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"sdlc-coordinator/internal/configuration"

	"gorm.io/gorm"
)

// Record 是组织段配置在租户库中的存储形态：每库至多一行（固定主键 1），
// revision 单调递增，是热生效消费点的版本锚点。密钥内联在载荷里，
// 与平台快照的持有方式一致。
type Record struct {
	ID          uint      `gorm:"primaryKey"`
	Revision    int64     `gorm:"not null"`
	PayloadJSON string    `gorm:"type:text;not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Record) TableName() string { return "tenant_runtime_config" }

// Config 复用系统段既有类型，不发明新形状。
type Config struct {
	CodeHost configuration.CodeHostConfig `json:"codehost"`
	IM       configuration.IMConfig       `json:"im"`
}

// ReworkAllowedIDs 把白名单字符串转为三平台 normalizer 消费的数值 id；
// 校验保证为正整数字符串，解析失败项静默跳过（防御旧数据）。
func (c Config) ReworkAllowedIDs() []int {
	ids := make([]int, 0, len(c.CodeHost.ReworkAllowedUserIDs))
	for _, raw := range c.CodeHost.ReworkAllowedUserIDs {
		value := 0
		for _, r := range raw {
			if r < '0' || r > '9' {
				value = -1
				break
			}
			value = value*10 + int(r-'0')
		}
		if value > 0 {
			ids = append(ids, value)
		}
	}
	return ids
}

// Store 提供组织段配置在单个库句柄上的读写；无状态，可按请求构造。
type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) Migrate() error { return s.db.AutoMigrate(&Record{}) }

// Load 返回当前配置；无行时 found=false（组织段未配置是合法状态）。
func (s *Store) Load(ctx context.Context) (Config, int64, bool, error) {
	var record Record
	err := s.db.WithContext(ctx).First(&record, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Config{}, 0, false, nil
	}
	if err != nil {
		return Config{}, 0, false, fmt.Errorf("load tenant runtime config: %w", err)
	}
	var config Config
	if err := json.Unmarshal([]byte(record.PayloadJSON), &config); err != nil {
		return Config{}, 0, false, fmt.Errorf("decode tenant runtime config: %w", err)
	}
	return config, record.Revision, true, nil
}

// Save 整行覆盖保存（校验由调用方先行完成），revision 在事务内自增。
func (s *Store) Save(ctx context.Context, config Config) (int64, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return 0, fmt.Errorf("encode tenant runtime config: %w", err)
	}
	var revision int64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Record
		err := tx.First(&current, 1).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			record := Record{ID: 1, Revision: 1, PayloadJSON: string(payload)}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
			revision = 1
			return nil
		case err != nil:
			return err
		default:
			revision = current.Revision + 1
			return tx.Model(&Record{}).Where("id = ?", current.ID).Updates(map[string]any{
				"revision":     revision,
				"payload_json": string(payload),
				"updated_at":   time.Now().UTC(),
			}).Error
		}
	})
	if err != nil {
		return 0, fmt.Errorf("save tenant runtime config: %w", err)
	}
	return revision, nil
}
