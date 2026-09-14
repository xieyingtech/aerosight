package inspection

import (
	"context"
	"database/sql"
	"errors"
)

// LockAlertConnector serializes policy changes with the connector projector,
// including the first policy insert (where a policy row cannot be locked yet).
func LockAlertConnector(ctx context.Context, tx *sql.Tx, projectID int, connectorID int64) (int, error) {
	var team int
	err := tx.QueryRowContext(ctx, "select team_id from device_adapters where project_id=$1 and id=$2 and adapter_type='dji-flighthub2' for update", projectID, connectorID).Scan(&team)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("INSPECTION_CONNECTOR_SCOPE_INVALID")
	}
	return team, err
}

func SetAlertPolicy(ctx context.Context, tx *sql.Tx, projectID int, connectorID int64, managed bool) error {
	team, err := LockAlertConnector(ctx, tx, projectID, connectorID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `insert into inspection_connector_policies(project_id,team_id,connector_instance_id,task_managed_alerts)
 values($1,$2,$3,$4) on conflict(project_id,connector_instance_id) do update set task_managed_alerts=excluded.task_managed_alerts`, projectID, team, connectorID, managed)
	return err
}

// ClassifyAlertFlight must run while holding the connector lock. A previously
// held or Task-managed flight stays protected when the connector default changes.
func ClassifyAlertFlight(ctx context.Context, tx *sql.Tx, projectID int, connectorID int64, flightID string) (string, error) {
	if !validIdentity(flightID) {
		return "", errors.New("INSPECTION_FLIGHT_ID_INVALID")
	}
	var ownership string
	err := tx.QueryRowContext(ctx, "select ownership from inspection_flight_ownership where project_id=$1 and connector_instance_id=$2 and remote_flight_id=$3", projectID, connectorID, flightID).Scan(&ownership)
	if err == nil {
		return ownership, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var managed bool
	if err = tx.QueryRowContext(ctx, "select coalesce((select task_managed_alerts from inspection_connector_policies where project_id=$1 and connector_instance_id=$2),false)", projectID, connectorID).Scan(&managed); err != nil {
		return "", err
	}
	if !managed {
		return "legacy", nil
	}
	_, err = tx.ExecContext(ctx, "insert into inspection_flight_ownership(project_id,connector_instance_id,remote_flight_id,ownership) values($1,$2,$3,'pending') on conflict do nothing", projectID, connectorID, flightID)
	return "pending", err
}

// ReleaseLegacyFlight is an explicit classification, not the connector default.
// Task-owned flights cannot be released through this path to bypass assessment.
func ReleaseLegacyFlight(ctx context.Context, tx *sql.Tx, projectID int, connectorID int64, flightID string) error {
	if !validIdentity(flightID) {
		return errors.New("INSPECTION_FLIGHT_ID_INVALID")
	}
	if _, err := LockAlertConnector(ctx, tx, projectID, connectorID); err != nil {
		return err
	}
	var known bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from inspection_flight_ownership where project_id=$1 and connector_instance_id=$2 and remote_flight_id=$3)
 or exists(select 1 from connector_remote_resources where project_id=$1 and connector_instance_id=$2 and resource_kind='flight-task' and remote_id=$3)`, projectID, connectorID, flightID).Scan(&known); err != nil {
		return err
	}
	if !known {
		return errors.New("INSPECTION_FLIGHT_SCOPE_INVALID")
	}
	result, err := tx.ExecContext(ctx, `insert into inspection_flight_ownership(project_id,connector_instance_id,remote_flight_id,ownership)
 values($1,$2,$3,'legacy') on conflict(project_id,connector_instance_id,remote_flight_id) do update set ownership='legacy'
 where inspection_flight_ownership.ownership<>'task'`, projectID, connectorID, flightID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return errors.New("INSPECTION_TASK_OWNERSHIP_IMMUTABLE")
	}
	return err
}

// ClaimAlertFlight records business ownership without changing any projected
// Run or legacy issue. Repeated read-only analysis preserves the initial owner.
func ClaimAlertFlight(ctx context.Context, tx *sql.Tx, projectID int, connectorID int64, flightID string, runID int64) error {
	if !validIdentity(flightID) {
		return errors.New("INSPECTION_FLIGHT_ID_INVALID")
	}
	if _, err := LockAlertConnector(ctx, tx, projectID, connectorID); err != nil {
		return err
	}
	var business bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from task_runs run join task_versions version on version.id=run.task_version_id and version.project_id=run.project_id
 where run.project_id=$1 and run.id=$2 and version.dsl_version='aerosight/v2')`, projectID, runID).Scan(&business); err != nil {
		return err
	}
	if !business {
		return errors.New("INSPECTION_BUSINESS_RUN_SCOPE_INVALID")
	}
	_, err := tx.ExecContext(ctx, `insert into inspection_flight_ownership(project_id,connector_instance_id,remote_flight_id,ownership,task_run_id)
 values($1,$2,$3,'task',$4) on conflict(project_id,connector_instance_id,remote_flight_id) do update set ownership='task',task_run_id=excluded.task_run_id
 where inspection_flight_ownership.ownership<>'task'`, projectID, connectorID, flightID, runID)
	return err
}
