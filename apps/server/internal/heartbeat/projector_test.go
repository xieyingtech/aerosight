package heartbeat

import (
	"testing"
	"time"
)

func TestEvaluateTransitionsWithControlledTime(t *testing.T) {
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	interval := 30 * time.Second
	tests := []struct {
		name    string
		now     time.Time
		last    *time.Time
		closed  *time.Time
		quality *float64
		status  string
		reason  string
	}{
		{"never observed", base, nil, nil, nil, "unknown", "awaiting_heartbeat"},
		{"fresh", base.Add(30 * time.Second), timePointer(base), nil, nil, "online", "heartbeat_fresh"},
		{"stale", base.Add(61 * time.Second), timePointer(base), nil, nil, "degraded", "heartbeat_stale"},
		{"expired", base.Add(121 * time.Second), timePointer(base), nil, nil, "offline", "heartbeat_timeout"},
		{"closed", base, timePointer(base), timePointer(base), nil, "offline", "session_closed"},
		{"clock skew", base, timePointer(base.Add(31 * time.Second)), nil, nil, "degraded", "heartbeat_clock_skew"},
		{"weak link", base, timePointer(base), nil, floatPointer(0.1), "degraded", "low_link_quality"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := Evaluate(test.now, test.last, test.closed, interval, test.quality, false)
			if projection.Status != test.status || projection.Reason != test.reason {
				t.Fatalf("got %s/%s want %s/%s", projection.Status, projection.Reason, test.status, test.reason)
			}
		})
	}
}

func TestStaleHeartbeatNeverLooksRealtime(t *testing.T) {
	last := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	projection := Evaluate(last.Add(5*time.Minute), &last, nil, 30*time.Second, nil, false)
	if projection.Status == "online" {
		t.Fatal("stale heartbeat must not be projected as online")
	}
}

func timePointer(value time.Time) *time.Time { return &value }
func floatPointer(value float64) *float64    { return &value }
