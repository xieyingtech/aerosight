package flighthub

import (
	"aerosight/server/internal/inspection"
	"context"
	"database/sql"
	"errors"
)

// Run before projecting the remote identity, under the same transaction. A
// canceled business Run still owns its late results; ownership is not execution.
func claimInspectionActionFlight(ctx context.Context, tx *sql.Tx, job FlightActionJob, remoteID string) error {
	var businessRun int64
	var knownRemote sql.NullString
	err := tx.QueryRowContext(ctx, `select b.business_run_id,r.remote_id from inspection_flight_bindings b join connector_action_jobs j on j.id=b.action_job_id and j.project_id=b.project_id left join connector_remote_resources r on r.id=j.remote_result_resource_id and r.project_id=j.project_id where b.project_id=$1 and b.action_job_id=$2 and b.connector_instance_id=$3 and b.flight_run_id=$4 for update of j`, job.ProjectID, job.ID, job.ConnectorInstanceID, job.TaskRunID).Scan(&businessRun, &knownRemote)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if knownRemote.Valid && knownRemote.String != remoteID {
		return errors.New("INSPECTION_FLIGHT_IDENTITY_CHANGED")
	}
	if _, err = inspection.LockAlertConnector(ctx, tx, job.ProjectID, job.ConnectorInstanceID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `insert into inspection_flight_ownership(project_id,connector_instance_id,remote_flight_id,ownership,task_run_id)
 values($1,$2,$3,'task',$4) on conflict(project_id,connector_instance_id,remote_flight_id) do update set ownership='task',task_run_id=excluded.task_run_id
 where inspection_flight_ownership.ownership='pending' or (inspection_flight_ownership.ownership='task' and inspection_flight_ownership.task_run_id=excluded.task_run_id)`, job.ProjectID, job.ConnectorInstanceID, remoteID, businessRun)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("INSPECTION_FLIGHT_OWNERSHIP_CONFLICT")
	}
	return nil
}
