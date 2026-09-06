package httpapi

import (
	"context"
	"log/slog"
)

// slog-gin includes query, params, referer and error text even with body/header
// logging disabled. Filter its output, without modifying the actual request.
type accessLogHandler struct{ next slog.Handler }

func (h accessLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}
func (h accessLogHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, "HTTP request", record.PC)
	record.Attrs(func(a slog.Attr) bool {
		if a, ok := accessLogAttribute(a); ok {
			clean.AddAttrs(a)
		}
		return true
	})
	return h.next.Handle(ctx, clean)
}
func (h accessLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		if a, ok := accessLogAttribute(a); ok {
			clean = append(clean, a)
		}
	}
	return accessLogHandler{h.next.WithAttrs(clean)}
}
func (h accessLogHandler) WithGroup(name string) slog.Handler {
	return accessLogHandler{h.next.WithGroup(name)}
}
func accessLogAttribute(a slog.Attr) (slog.Attr, bool) {
	if a.Key == "id" && a.Value.Kind() == slog.KindString {
		return a, true
	}
	if (a.Key != "request" && a.Key != "response") || a.Value.Kind() != slog.KindGroup {
		return slog.Attr{}, false
	}
	allowed := map[string]bool{"time": true, "method": true, "route": true, "length": true}
	if a.Key == "response" {
		allowed = map[string]bool{"time": true, "latency": true, "status": true, "length": true}
	}
	attrs := []slog.Attr{}
	for _, field := range a.Value.Group() {
		if allowed[field.Key] {
			attrs = append(attrs, field)
		}
	}
	return slog.Attr{Key: a.Key, Value: slog.GroupValue(attrs...)}, true
}
