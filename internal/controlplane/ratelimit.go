package controlplane

import (
	"sync"
	"time"
)

const (
	defaultRateLimitWindow      = 15 * time.Minute
	defaultRateLimitMaxFailures = 10
	// defaultRateLimitMaxKeys 限制失败表条目数：键为「用户名+IP」，
	// 无上限时可被伪造请求撑爆内存；超限先清过期，仍满则整体重置。
	defaultRateLimitMaxKeys = 4096
)

// loginRateLimiter 按「用户名+IP」组合做滑动窗口失败计数：
// 窗口内失败达到上限即拒绝（含正确密码），成功登录清零。进程内实现，单实例
// 形态下与部署形态一致；多实例部署时以控制面库承载是后续演进。
type loginRateLimiter struct {
	mu          sync.Mutex
	window      time.Duration
	maxFailures int
	maxKeys     int
	failures    map[string][]time.Time
	now         func() time.Time
}

func newLoginRateLimiter(window time.Duration, maxFailures, maxKeys int) *loginRateLimiter {
	if maxKeys < 1 {
		maxKeys = defaultRateLimitMaxKeys
	}
	return &loginRateLimiter{
		window:      window,
		maxFailures: maxFailures,
		maxKeys:     maxKeys,
		failures:    map[string][]time.Time{},
		now:         time.Now,
	}
}

func (limiter *loginRateLimiter) blocked(key string) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.pruneLocked(key)
	return len(limiter.failures[key]) >= limiter.maxFailures
}

func (limiter *loginRateLimiter) record(key string) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if _, exists := limiter.failures[key]; !exists && len(limiter.failures) >= limiter.maxKeys {
		// 容量淘汰：按「最新失败时间最旧」的键逐个驱逐，避免
		// 全表重置把受害者正在累积的锁定计数一并冲刷掉。
		for len(limiter.failures) >= limiter.maxKeys {
			if !limiter.evictOldestLocked() {
				break
			}
		}
	}
	limiter.failures[key] = append(limiter.failures[key], limiter.now())
}

func (limiter *loginRateLimiter) reset(key string) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	delete(limiter.failures, key)
}

func (limiter *loginRateLimiter) prune(key string) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.pruneLocked(key)
}

// evictOldestLocked 删除「最新失败时间最旧」的一个键；调用方须已持锁。
// 返回是否发生了驱逐（表为空时返回 false 防死循环）。
func (limiter *loginRateLimiter) evictOldestLocked() bool {
	oldestKey := ""
	var oldestAt time.Time
	for key, stamps := range limiter.failures {
		if len(stamps) == 0 {
			oldestKey = key
			break
		}
		latest := stamps[len(stamps)-1]
		if oldestKey == "" || latest.Before(oldestAt) {
			oldestKey = key
			oldestAt = latest
		}
	}
	if oldestKey == "" {
		return false
	}
	delete(limiter.failures, oldestKey)
	return true
}

// pruneLocked 清理单键的过期窗口记录；调用方须已持锁。
func (limiter *loginRateLimiter) pruneLocked(key string) {
	cutoff := limiter.now().Add(-limiter.window)
	kept := limiter.failures[key][:0]
	for _, at := range limiter.failures[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(limiter.failures, key)
		return
	}
	limiter.failures[key] = kept
}
