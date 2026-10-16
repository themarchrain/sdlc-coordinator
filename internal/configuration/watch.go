package configuration

import (
	"context"
	"errors"
	"time"
)

func WatchForNewerRevision(ctx context.Context, repository *Repository, revision uint64, shutdown ShutdownRequester, interval time.Duration, onError func(error)) {
	if interval <= 0 {
		interval = time.Second
	}
	check := func() bool {
		latest, err := repository.Latest(ctx)
		if err != nil {
			if !errors.Is(err, ErrSnapshotNotFound) && onError != nil {
				onError(err)
			}
			return false
		}
		if latest.Revision > revision {
			shutdown.RequestShutdown()
			return true
		}
		return false
	}
	if check() {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if check() {
				return
			}
		}
	}
}
