package httpapi

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// The upstream store has no context-aware cleanup. Own its lifecycle here while
// retaining the upstream store's persistence format and expiry semantics.
func startSessionCleanup(interval, timeout time.Duration, remove func(context.Context) error, logger *slog.Logger) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				operation, stop := context.WithTimeout(ctx, timeout)
				err := remove(operation)
				stop()
				if err != nil && ctx.Err() == nil {
					logger.Error("Session cleanup failed")
				}
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(cancel); <-done }
}
