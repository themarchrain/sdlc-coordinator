package controlplane

import (
	"context"
	"time"
)

const (
	defaultRateLimitWindow      = 15 * time.Minute
	defaultRateLimitMaxFailures = 10
	// loginFailureBatchLimit 是单次清理的删除上限：约束 DELETE 的持锁时间，
	// 残留旧行由后续记账轮次继续清理。
	loginFailureBatchLimit = 1000
)

// LoginFailure 是登录限流的共享存储行：键为用户名+IP 或纯用户名,
// 每次失败一行。行存控制面库，双实例共享同一失败配额（轮流访问不能
// 扩大原配额）；窗口判定用库内时钟（MySQL NOW(6)），跨实例时钟偏移免疫。
type LoginFailure struct {
	ID       uint      `gorm:"primaryKey"`
	Key      string    `gorm:"index:idx_login_failures_key_time,priority:1;not null;size:255"`
	FailedAt time.Time `gorm:"index:idx_login_failures_key_time,priority:2;index:idx_login_failures_failed_at;not null"`
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

// loginFailureKeyCap 是单键失败行的保留上限:超过 maxFailures 后 blocked
// 恒真,多余历史行对判定无影响;裁剪保留最新行,攻击者对单键高频失败时
// 表行数有界。
func loginFailureKeyCap(maxFailures int) int {
	if maxFailures <= 0 {
		return defaultRateLimitMaxFailures * 2
	}
	return maxFailures * 2
}

func (limiter *loginRateLimiter) record(key string) {
	now := time.Now()
	if err := limiter.repository.RecordLoginFailure(context.Background(), key, now); err != nil {
		// 记账失败不阻断登录错误路径:下一轮失败重试记账,最坏情况是
		// 攻击者少计一次;不因限流器故障把凭据错误误报成 500。
		return
	}
	// 容量治理两件套（都带界）:窗口外旧行分批删除（单批上限约束持锁时间,
	// 清理条件走 failed_at 索引）;单键行数裁剪（匿名来源可用不同键放大总
	// 行数,单键上限保证任一键不无界堆积）。
	_ = limiter.repository.DeleteLoginFailuresBeforeBounded(context.Background(), now.Add(-limiter.window), loginFailureBatchLimit)
	_ = limiter.repository.CapKeyLoginFailures(context.Background(), key, loginFailureKeyCap(limiter.maxFailures))
}

func (limiter *loginRateLimiter) reset(key string) {
	_ = limiter.repository.DeleteLoginFailures(context.Background(), key)
}

func (limiter *loginRateLimiter) prune(key string) {
	// DB 后端的窗口过期在读取时按库内时间过滤,无需显式清理。
}
