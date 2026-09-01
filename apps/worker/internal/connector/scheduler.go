package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aerosight/worker/internal/outbox"
)

var (
	ErrConnectorLeaseUnavailable = errors.New("CONNECTOR_LEASE_UNAVAILABLE")
	ErrConnectorLeaseLost        = errors.New("CONNECTOR_LEASE_LOST")
	safeCodePattern              = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

type terminalConnectorSyncError struct{ cause error }

func (err *terminalConnectorSyncError) Error() string { return err.cause.Error() }
func (err *terminalConnectorSyncError) Unwrap() error { return err.cause }

type SyncRunner interface {
	Run(context.Context, Instance, DiscoveryMode) (SyncApplyResult, error)
}

type SyncOutcomeStore interface {
	Succeeded(context.Context, InstanceLease, string, SyncApplyResult) error
	Failed(context.Context, InstanceLease, string, DiscoveryMode, string) error
}

type MetricRecorder interface {
	Record(string, float64, map[string]string) error
}

type SQLSyncOutcomeStore struct{ db *sql.DB }

func NewSQLSyncOutcomeStore(db *sql.DB) *SQLSyncOutcomeStore { return &SQLSyncOutcomeStore{db: db} }

func (store *SQLSyncOutcomeStore) Succeeded(
	ctx context.Context, lease InstanceLease, owner string, result SyncApplyResult,
) error {
	health, err := json.Marshal(map[string]any{
		"ok": true, "code": "sync_succeeded", "runId": result.RunID,
		"discovered": result.Discovered, "missing": result.Missing,
	})
	if err != nil {
		return err
	}
	updated, err := store.db.ExecContext(ctx, `
		update device_adapters
		   set status='connected',last_health_json=$5,last_checked_at=now(),last_connected_at=now(),
		       lease_owner=null,lease_expires_at=null,updated_at=now()
		 where id=$1 and project_id=$2 and connection_epoch=$3 and lease_owner=$4 and lease_expires_at>=now()`,
		lease.ID, lease.ProjectID, lease.Epoch, owner, health)
	if err != nil {
		return err
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConnectorLeaseLost
	}
	return nil
}

func (store *SQLSyncOutcomeStore) Failed(
	ctx context.Context, lease InstanceLease, owner string, mode DiscoveryMode, code string,
) (returnedErr error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	var teamID int
	var scope, cursor []byte
	err = tx.QueryRowContext(ctx, `
		select team_id,discovery_scope_json,sync_cursor_json
		  from device_adapters
		 where id=$1 and project_id=$2 and connection_epoch=$3 and lease_owner=$4 and lease_expires_at>=now()
		 for update`, lease.ID, lease.ProjectID, lease.Epoch, owner).Scan(&teamID, &scope, &cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConnectorLeaseLost
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		insert into connector_sync_runs (
		  project_id,team_id,connector_instance_id,discovery_mode,status,scope_json,
		  cursor_before_json,cursor_after_json,error_code,started_at,finished_at
		) values ($1,$2,$3,$4,'failed',$5,$6,$6,$7,now(),now())`,
		lease.ProjectID, teamID, lease.ID, mode, scope, cursor, strings.ToUpper(code)); err != nil {
		return err
	}
	status := "degraded"
	if code == "credential_invalid" || code == "credential_unavailable" || code == "scope_forbidden" || code == "scope_not_found" {
		status = "failed"
	}
	health, err := json.Marshal(map[string]any{"ok": false, "code": code})
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		update device_adapters
		   set status=$5,last_health_json=$6,last_checked_at=now(),
		       lease_owner=null,lease_expires_at=null,updated_at=now()
		 where id=$1 and project_id=$2 and connection_epoch=$3 and lease_owner=$4`,
		lease.ID, lease.ProjectID, lease.Epoch, owner, status, health); err != nil {
		return err
	}
	return tx.Commit()
}

type SchedulerConfig struct {
	Owner          string
	ConnectorKey   string
	Version        string
	PollInterval   time.Duration
	JitterWindow   time.Duration
	ReconcileEvery time.Duration
	LeaseDuration  time.Duration
	RenewEvery     time.Duration
	BatchSize      int
	Now            func() time.Time
	Jitter         func(time.Duration) time.Duration
	Logger         *slog.Logger
	Metrics        MetricRecorder
}

type Scheduler struct {
	leases   LeaseRepository
	runner   SyncRunner
	outcomes SyncOutcomeStore
	config   SchedulerConfig
}

func NewScheduler(leases LeaseRepository, runner SyncRunner, outcomes SyncOutcomeStore, config SchedulerConfig) (*Scheduler, error) {
	if leases == nil || runner == nil || outcomes == nil || config.Owner == "" || config.ConnectorKey == "" || config.Version == "" ||
		config.PollInterval <= 0 || config.JitterWindow < 0 || config.ReconcileEvery <= 0 || config.LeaseDuration <= 0 ||
		config.RenewEvery <= 0 || config.RenewEvery >= config.LeaseDuration || config.BatchSize <= 0 {
		return nil, errors.New("connector scheduler configuration is invalid")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Jitter == nil {
		config.Jitter = func(window time.Duration) time.Duration {
			if window <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(window) + 1))
		}
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	return &Scheduler{leases: leases, runner: runner, outcomes: outcomes, config: config}, nil
}

func connectorErrorCode(err error) string {
	if err == nil {
		return "sync_failed"
	}
	var safe interface{ ConnectorSafeCode() string }
	if errors.As(err, &safe) && safeCodePattern.MatchString(safe.ConnectorSafeCode()) {
		return safe.ConnectorSafeCode()
	}
	text := strings.ToLower(strings.TrimPrefix(err.Error(), "DJI_FLIGHTHUB_"))
	text = strings.ReplaceAll(text, "-", "_")
	if safeCodePattern.MatchString(text) {
		return text
	}
	return "sync_failed"
}

func connectorMetricOutcome(err error) string {
	if err == nil {
		return "succeeded"
	}
	if errors.Is(err, ErrConnectorLeaseLost) {
		return "lease_lost"
	}
	switch connectorErrorCode(err) {
	case "rate_limited":
		return "rate_limited"
	case "credential_invalid", "credential_unavailable":
		return "credential_invalid"
	case "schema_incompatible":
		return "schema_incompatible"
	case "directory_limit_reached", "response_too_large":
		return "directory_incomplete"
	default:
		return "failed"
	}
}

func terminalConnectorErrorCode(code string) bool {
	switch code {
	case "credential_invalid", "credential_unavailable", "scope_forbidden", "scope_not_found":
		return true
	default:
		return false
	}
}

func (scheduler *Scheduler) recordSync(duration time.Duration, err error) {
	if scheduler.config.Metrics == nil {
		return
	}
	_ = scheduler.config.Metrics.Record("aerosight_connector_sync_total", 1, map[string]string{
		"connector": "dji_flighthub2", "outcome": connectorMetricOutcome(err),
	})
	durationOutcome := "succeeded"
	if err != nil {
		durationOutcome = "failed"
	}
	_ = scheduler.config.Metrics.Record("aerosight_connector_sync_duration_seconds", duration.Seconds(), map[string]string{
		"connector": "dji_flighthub2", "outcome": durationOutcome,
	})
}

func (scheduler *Scheduler) executeLease(ctx context.Context, lease InstanceLease) (result SyncApplyResult, returnedErr error) {
	startedAt := time.Now()
	defer func() { scheduler.recordSync(time.Since(startedAt), returnedErr) }()
	syncContext, cancel := context.WithCancel(ctx)
	renewed := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(scheduler.config.RenewEvery)
		defer ticker.Stop()
		for {
			select {
			case <-syncContext.Done():
				renewed <- nil
				return
			case <-ticker.C:
				ok, err := scheduler.leases.Renew(syncContext, lease, scheduler.config.Owner, scheduler.config.LeaseDuration)
				if err != nil || !ok {
					if err == nil {
						err = ErrConnectorLeaseLost
					}
					cancel()
					renewed <- err
					return
				}
			}
		}
	}()
	result, syncErr := scheduler.runner.Run(syncContext, lease.Instance, DiscoveryPoll)
	cancel()
	renewErr := <-renewed
	if renewErr != nil {
		syncErr = renewErr
	}
	if syncErr != nil {
		code := connectorErrorCode(syncErr)
		if outcomeErr := scheduler.outcomes.Failed(ctx, lease, scheduler.config.Owner, DiscoveryPoll, code); outcomeErr != nil {
			return result, errors.Join(syncErr, outcomeErr)
		}
		if terminalConnectorErrorCode(code) {
			return result, &terminalConnectorSyncError{cause: syncErr}
		}
		return result, syncErr
	}
	if err := scheduler.outcomes.Succeeded(ctx, lease, scheduler.config.Owner, result); err != nil {
		return result, err
	}
	return result, nil
}

type syncRequest struct {
	ConnectorInstanceID string `json:"connectorInstanceId"`
	ConnectorKey        string `json:"connectorKey"`
	DiscoveryMode       string `json:"discoveryMode"`
	Trigger             string `json:"trigger"`
}

func (scheduler *Scheduler) OutboxHandler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	var request syncRequest
	if event.ProjectID <= 0 || event.TeamID <= 0 || json.Unmarshal(event.Payload, &request) != nil ||
		request.ConnectorKey != scheduler.config.ConnectorKey || request.DiscoveryMode != string(DiscoveryPoll) ||
		(request.Trigger != "initial" && request.Trigger != "manual" && request.Trigger != "credential-update" && request.Trigger != "capability-probe") {
		return errors.New("CONNECTOR_SYNC_REQUEST_INVALID")
	}
	instanceID, err := strconv.ParseInt(request.ConnectorInstanceID, 10, 64)
	if err != nil || instanceID <= 0 {
		return errors.New("CONNECTOR_SYNC_REQUEST_INVALID")
	}
	lease, claimed, err := scheduler.leases.ClaimInstance(
		ctx, scheduler.config.Owner, event.ProjectID, instanceID,
		scheduler.config.ConnectorKey, scheduler.config.Version, scheduler.config.LeaseDuration,
	)
	if err != nil {
		return err
	}
	if !claimed || lease.TeamID != event.TeamID {
		if claimed {
			_ = scheduler.leases.Release(ctx, lease, scheduler.config.Owner)
		}
		return ErrConnectorLeaseUnavailable
	}
	_, err = scheduler.executeLease(ctx, lease)
	var terminalError *terminalConnectorSyncError
	if errors.As(err, &terminalError) {
		return nil
	}
	return err
}

func (scheduler *Scheduler) ReconcileOnce(ctx context.Context) (int, error) {
	backlog, backlogErr := scheduler.leases.Backlog(ctx, scheduler.config.ConnectorKey, scheduler.config.Version)
	if backlogErr != nil {
		return 0, backlogErr
	}
	if scheduler.config.Metrics != nil {
		_ = scheduler.config.Metrics.Record("aerosight_connector_sync_backlog", float64(backlog), map[string]string{"connector": "dji_flighthub2"})
	}
	jitter := scheduler.config.Jitter(scheduler.config.JitterWindow)
	if jitter < 0 || jitter > scheduler.config.JitterWindow {
		return 0, fmt.Errorf("connector scheduler jitter is out of range: %s", jitter)
	}
	dueBefore := scheduler.config.Now().Add(-scheduler.config.PollInterval - jitter)
	leases, err := scheduler.leases.ClaimDue(
		ctx, scheduler.config.Owner, scheduler.config.ConnectorKey, scheduler.config.Version,
		dueBefore, scheduler.config.BatchSize, scheduler.config.LeaseDuration,
	)
	if err != nil {
		return 0, err
	}
	var syncErrors []error
	for _, lease := range leases {
		if _, syncErr := scheduler.executeLease(ctx, lease); syncErr != nil {
			syncErrors = append(syncErrors, syncErr)
		}
	}
	return len(leases), errors.Join(syncErrors...)
}

func (scheduler *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(scheduler.config.ReconcileEvery)
	defer ticker.Stop()
	for {
		if _, err := scheduler.ReconcileOnce(ctx); err != nil && ctx.Err() == nil {
			scheduler.config.Logger.Error("connector scheduler reconciliation failed",
				"connector_key", scheduler.config.ConnectorKey, "error", connectorErrorCode(err))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
