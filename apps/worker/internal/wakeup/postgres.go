package wakeup

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

func Postgres(ctx context.Context, databaseURL string, logger *slog.Logger) <-chan struct{} {
	wake := make(chan struct{}, 1)
	go func() {
		defer close(wake)
		connection, err := pgx.Connect(ctx, databaseURL)
		if err != nil {
			logger.Warn("outbox notification connection unavailable; polling remains active", "error", err.Error())
			return
		}
		defer connection.Close(context.Background())
		if _, err := connection.Exec(ctx, "listen aerosight_outbox"); err != nil {
			logger.Warn("outbox LISTEN unavailable; polling remains active", "error", err.Error())
			return
		}
		for {
			if _, err := connection.WaitForNotification(ctx); err != nil {
				if ctx.Err() == nil {
					logger.Warn("outbox notification listener stopped; polling remains active", "error", err.Error())
				}
				return
			}
			select {
			case wake <- struct{}{}:
			default:
			}
		}
	}()
	return wake
}
