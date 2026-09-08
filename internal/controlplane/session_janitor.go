package controlplane

import (
	"context"
	"time"
)

const defaultSessionSweepInterval = time.Hour

// PurgeExpiredSessions 删除已过期会话行，返回删除条数。
func (service *Service) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	return service.repository.DeleteExpiredSessions(ctx, service.now().UTC())
}

// RunSessionJanitor 周期性清理过期会话；启动时先扫一次。
func (service *Service) RunSessionJanitor(ctx context.Context) {
	service.runSessionJanitor(ctx, defaultSessionSweepInterval)
}

func (service *Service) runSessionJanitor(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultSessionSweepInterval
	}
	_, _ = service.PurgeExpiredSessions(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = service.PurgeExpiredSessions(ctx)
		}
	}
}
