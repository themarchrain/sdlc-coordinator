package controlplane

import (
	"context"
	"time"
)

const (
	defaultRateLimitWindow      = 15 * time.Minute
	defaultRateLimitMaxFailures = 10
)

// LoginFailure 是登录限流的共享存储行：键为用户名+IP 或纯用户名,
// 每次失败一行。行存控制面库，双实例共享同一失败配额（轮流访问不能
// 扩大原配额）；窗口判定用库内时钟（MySQL NOW(6)），跨实例时钟偏移免疫。
type LoginFailure struct {
	ID       uint      `gorm:"primaryKey"`
	Key      string    `gorm:"index:idx_login_failures_key_time,priority:1;not null;size:255"`
	FailedAt time.Time `gorm:"index:idx_login_failures_key_time,priority:2;not null"`
}

// loginRateLimiter 按用户名+IP 组合做滑动窗口失败计数:窗口内失败达到
// 上限即拒绝（含正确密码），成功登录清零。计数落控制面库，多实例部署共享
// 同一配额；窗口外旧行在记账时顺带清理（登录低频，代价可忽略）。
type loginRateLimiter struct {
	repository  *Repository
	window      time.Duration
	maxFailures int
}

func newLoginRateLimiter(repository *Repository, window time.Duration, maxFailures int) *loginRateLimiter {
	return &loginRateLimiter{
		repository:  repository,
		window:      window,
		maxFailures: maxFailures,
	}
}

func (limiter *loginRateLimiter) blocked(key string) bool {
	count, err := limiter.repository.CountRecentLoginFailures(context.Background(), key, limiter.window)
	if err != nil {
		// 读失败 fail-closed：锁死比放行安全（放行=爆破窗口）。
		return true
	}
	return count >= int64(limiter.maxFailures)
}

func (limiter *loginRateLimiter) record(key string) {
	now := time.Now()
	if err := limiter.repository.RecordLoginFailure(context.Background(), key, now); err != nil {
		// 记账失败不阻断登录错误路径:下一轮失败重试记账,最坏情况是
		// 攻击者少计一次;不因限流器故障把凭据错误误报成 500。
		return
	}
	_ = limiter.repository.DeleteLoginFailuresBefore(context.Background(), now.Add(-limiter.window))
}

func (limiter *loginRateLimiter) reset(key string) {
	_ = limiter.repository.DeleteLoginFailures(context.Background(), key)
}

func (limiter *loginRateLimiter) prune(key string) {
	// DB 后端的窗口过期在读取时按库内时间过滤,无需显式清理。
}
