package orgconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// Provider 是引擎侧组织段配置的读通式入口：每次 Get 都读当前行的
// revision，未变化时复用上次解析结果；读失败时返回上次已知配置与
// error（调用方记日志降级，不让一次抖动把通知链拍灭）。
type Provider struct {
	db *gorm.DB

	mu       sync.Mutex
	revision int64
	config   Config
}

func NewProvider(db *gorm.DB) *Provider { return &Provider{db: db} }

func (p *Provider) Get(ctx context.Context) (Config, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var record Record
	err := p.db.WithContext(ctx).First(&record, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		p.revision, p.config = 0, Config{}
		return Config{}, nil
	}
	if err != nil {
		return p.config, fmt.Errorf("load tenant runtime config: %w", err)
	}
	if record.Revision == p.revision {
		return p.config, nil
	}
	var config Config
	if err := json.Unmarshal([]byte(record.PayloadJSON), &config); err != nil {
		return p.config, fmt.Errorf("decode tenant runtime config: %w", err)
	}
	p.revision, p.config = record.Revision, config
	return config, nil
}
