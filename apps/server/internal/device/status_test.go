package device

import (
	"errors"
	"testing"
	"time"
)

func TestEveryDeviceClassUsesTheSameStatusContract(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	for _, class := range []Class{ClassDock, ClassAircraft, ClassRobot, ClassCamera, ClassSensor, ClassGateway} {
		t.Run(string(class), func(t *testing.T) {
			projection, err := ApplyStatusObservation(StatusProjection{}, StatusObservation{
				ObservedAt: now.Add(-10 * time.Second), ReceivedAt: now, RawReference: "raw/status/123",
			}, 30*time.Second)
			if err != nil || projection.Status != StatusOnline || projection.Freshness != FreshnessFresh || projection.RawReference == "" {
				t.Fatalf("class %s received a divergent status contract: %+v err=%v", class, projection, err)
			}
		})
	}
}

func TestStatusProjectionTimeoutAndDisconnect(t *testing.T) {
	base := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	stale := EvaluateStatus(base.Add(61*time.Second), &base, nil, 30*time.Second, nil, false, "raw/1")
	if stale.Status != StatusDegraded || stale.Freshness != FreshnessStale {
		t.Fatalf("stale data looked current: %+v", stale)
	}
	expired := EvaluateStatus(base.Add(121*time.Second), &base, nil, 30*time.Second, nil, false, "raw/1")
	if expired.Status != StatusOffline || expired.Freshness != FreshnessExpired {
		t.Fatalf("expired data looked current: %+v", expired)
	}
	disconnected := EvaluateStatus(base.Add(10*time.Second), &base, &base, 30*time.Second, nil, false, "raw/2")
	if disconnected.Status != StatusOffline || disconnected.Reason != "session_closed" || disconnected.Freshness != FreshnessFresh {
		t.Fatalf("explicit disconnect was not retained with data freshness: %+v", disconnected)
	}
}

func TestOutOfOrderStatusCannotOverwriteNewerProjection(t *testing.T) {
	base := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	newer, err := ApplyStatusObservation(StatusProjection{}, StatusObservation{ObservedAt: base.Add(time.Minute), ReceivedAt: base.Add(time.Minute)}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyStatusObservation(newer, StatusObservation{ObservedAt: base, ReceivedAt: base.Add(2 * time.Minute), Disconnected: true}, 30*time.Second)
	if !errors.Is(err, ErrOutOfOrderStatus) || result.Status != newer.Status || !result.ObservedAt.Equal(*newer.ObservedAt) {
		t.Fatalf("late disconnect overwrote newer status: result=%+v err=%v", result, err)
	}
}
