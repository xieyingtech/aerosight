package device

import (
	"errors"
	"time"
)

type Status string

const (
	StatusOnline   Status = "online"
	StatusDegraded Status = "degraded"
	StatusOffline  Status = "offline"
	StatusUnknown  Status = "unknown"
)

type Freshness string

const (
	FreshnessFresh   Freshness = "fresh"
	FreshnessStale   Freshness = "stale"
	FreshnessExpired Freshness = "expired"
	FreshnessUnknown Freshness = "unknown"
)

var ErrOutOfOrderStatus = errors.New("status observation is not newer than the current projection")

type StatusProjection struct {
	Status       Status
	Reason       string
	Freshness    Freshness
	ObservedAt   *time.Time
	ProjectedAt  time.Time
	RawReference string
}

type StatusObservation struct {
	ObservedAt       time.Time
	ReceivedAt       time.Time
	Disconnected     bool
	ReportedDegraded bool
	LinkQuality      *float64
	RawReference     string
}

func normalizedInterval(interval time.Duration) time.Duration {
	if interval < 5*time.Second {
		return 30 * time.Second
	}
	return interval
}

func EvaluateStatus(now time.Time, observedAt, disconnectedAt *time.Time, interval time.Duration, linkQuality *float64, reportedDegraded bool, rawReference string) StatusProjection {
	interval = normalizedInterval(interval)
	projection := StatusProjection{Status: StatusUnknown, Reason: "awaiting_heartbeat", Freshness: FreshnessUnknown, ObservedAt: observedAt, ProjectedAt: now, RawReference: rawReference}
	if observedAt == nil {
		return projection
	}
	age := now.Sub(*observedAt)
	switch {
	case age < -interval:
		projection.Status = StatusDegraded
		projection.Reason = "heartbeat_clock_skew"
		projection.Freshness = FreshnessUnknown
	case age > 4*interval:
		projection.Status = StatusOffline
		projection.Reason = "heartbeat_timeout"
		projection.Freshness = FreshnessExpired
	case age > 2*interval:
		projection.Status = StatusDegraded
		projection.Reason = "heartbeat_stale"
		projection.Freshness = FreshnessStale
	default:
		projection.Status = StatusOnline
		projection.Reason = "heartbeat_fresh"
		projection.Freshness = FreshnessFresh
	}
	if disconnectedAt != nil {
		projection.Status = StatusOffline
		projection.Reason = "session_closed"
		return projection
	}
	if projection.Status != StatusOnline {
		return projection
	}
	if reportedDegraded {
		projection.Status = StatusDegraded
		projection.Reason = "device_reported_degraded"
		return projection
	}
	if linkQuality != nil && *linkQuality < 0.25 {
		projection.Status = StatusDegraded
		projection.Reason = "low_link_quality"
	}
	return projection
}

func ApplyStatusObservation(current StatusProjection, observation StatusObservation, interval time.Duration) (StatusProjection, error) {
	if observation.ObservedAt.IsZero() || observation.ReceivedAt.IsZero() {
		return current, errors.New("status observation requires observed and received timestamps")
	}
	if current.ObservedAt != nil && !observation.ObservedAt.After(*current.ObservedAt) {
		return current, ErrOutOfOrderStatus
	}
	var disconnectedAt *time.Time
	if observation.Disconnected {
		disconnectedAt = &observation.ObservedAt
	}
	return EvaluateStatus(observation.ReceivedAt, &observation.ObservedAt, disconnectedAt, interval,
		observation.LinkQuality, observation.ReportedDegraded, observation.RawReference), nil
}
