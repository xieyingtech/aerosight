package httptransport

import (
	"context"
	"time"
)

type operationTimeoutKey struct{}

func WithOperationTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, operationTimeoutKey{}, timeout)
}
func OperationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout, ok := ctx.Value(operationTimeoutKey{}).(time.Duration)
	if !ok || timeout <= 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}
