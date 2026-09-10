package orgconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// snapshot 是 Provider 的一份已解析缓存条目;updated_at 与 revision 共同
// 构成比较键,覆盖租户库被删后重建、revision 从头重计的边角。
type snapshot struct {
	revision  int64
	updatedAt time.Time
	config    Config
}

// Provider 是引擎侧组织段配置的读通式入口:每次 Get 都读当前行的
// revision,未变化时复用上次解析结果;查询与解析都在锁外进行,读失败时
// 返回上次已知配置与 error(调用方记日志降级,不让一次抖动把通知链
// 拍灭)。HasSnapshot 报告是否已有可降级的已知配置。
type Provider struct {
	db *gorm.DB

	publishMu sync.Mutex
	cached    atomic.Pointer[snapshot]
}

func NewProvider(db *gorm.DB) *Provider { return &Provider{db: db} }

// Get 返回当前配置;行不存在(组织段未配置)返回零值配置。
func (p *Provider) Get(ctx context.Context) (Config, error) {
	var record Record
	err := p.db.WithContext(ctx).First(&record, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 行不存在也经 publish 发布零值快照(而非直接 Store):并发下
		// 「读到不存在」与「读到新建行」乱序时,publish 的防回退比较
		// 保证零值条目不会覆盖更新的有效配置。稳态(行持续不存在,
		// 缓存已是零值键)跳过发布,未配置租户的热路径零分配。
		if current := p.cached.Load(); current == nil || current.revision != 0 || !current.updatedAt.IsZero() {
			p.publish(Record{}, Config{})
		}
		return Config{}, nil
	}
	if err != nil {
		return p.known(), fmt.Errorf("load tenant runtime config: %w", err)
	}
	if current := p.cached.Load(); current != nil && current.revision == record.Revision && current.updatedAt.Equal(record.UpdatedAt) {
		return current.config, nil
	}
	var config Config
	if err := json.Unmarshal([]byte(record.PayloadJSON), &config); err != nil {
		return p.known(), fmt.Errorf("decode tenant runtime config: %w", err)
	}
	p.publish(record, config)
	return config, nil
}

// HasSnapshot 报告是否已有一份已知配置可作为读失败时的降级来源;
// 冷启动后首次读取即失败的场景它为 false,调用方可据此选择拒收重试。
func (p *Provider) HasSnapshot() bool {
	return p.cached.Load() != nil
}

// known 返回上次已知配置;尚无任何快照时返回零值。
func (p *Provider) known() Config {
	if current := p.cached.Load(); current != nil {
		return current.config
	}
	return Config{}
}

// publish 仅在 (revision, updated_at) 不早于已缓存条目时发布,防止并发
// 解析乱序回退到旧配置;临界区只有一次指针交换。
func (p *Provider) publish(record Record, config Config) {
	next := &snapshot{revision: record.Revision, updatedAt: record.UpdatedAt, config: config}
	p.publishMu.Lock()
	defer p.publishMu.Unlock()
	if current := p.cached.Load(); current != nil && (current.revision > next.revision ||
		(current.revision == next.revision && current.updatedAt.After(next.updatedAt))) {
		return
	}
	p.cached.Store(next)
}
