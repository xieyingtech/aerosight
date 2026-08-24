package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("worker started")

	// Task consumers will be registered here once the first queue contract exists.
	<-ctx.Done()
	logger.Info("worker stopped")
}
