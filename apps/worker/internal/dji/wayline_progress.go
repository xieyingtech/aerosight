package dji

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"aerosight/worker/internal/adapter"
	"aerosight/worker/internal/outbox"
)

type WaylineProgress struct {
	FlightID            string `json:"flightId"`
	Status              string `json:"status"`
	Percent             int    `json:"percent"`
	CurrentStep         int    `json:"currentStep"`
	CurrentWaypoint     int    `json:"currentWaypoint"`
	WaylineMissionState int    `json:"waylineMissionState"`
	Result              int    `json:"result"`
}

func DecodeWaylineProgress(data json.RawMessage) (WaylineProgress, error) {
	var vendor struct {
		Result *int `json:"result"`
		Output struct {
			Status   string `json:"status"`
			Progress struct {
				CurrentStep int `json:"current_step"`
				Percent     int `json:"percent"`
			} `json:"progress"`
			Ext struct {
				FlightID            string `json:"flight_id"`
				CurrentWaypoint     int    `json:"current_waypoint_index"`
				WaylineMissionState int    `json:"wayline_mission_state"`
			} `json:"ext"`
		} `json:"output"`
	}
	if json.Unmarshal(data, &vendor) != nil || vendor.Result == nil || vendor.Output.Ext.FlightID == "" || vendor.Output.Status == "" || vendor.Output.Progress.Percent < 0 || vendor.Output.Progress.Percent > 100 {
		return WaylineProgress{}, errors.New("DJI_WAYLINE_PROGRESS_INVALID")
	}
	return WaylineProgress{
		FlightID: vendor.Output.Ext.FlightID, Status: vendor.Output.Status,
		Percent: vendor.Output.Progress.Percent, CurrentStep: vendor.Output.Progress.CurrentStep,
		CurrentWaypoint:     vendor.Output.Ext.CurrentWaypoint,
		WaylineMissionState: vendor.Output.Ext.WaylineMissionState, Result: *vendor.Result,
	}, nil
}

func (dispatcher *CommandDispatcher) EventHandler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var envelope adapter.UpstreamEnvelope
	if err := json.Unmarshal(event.Payload, &envelope); err != nil {
		return fmt.Errorf("decode DJI event envelope: %w", err)
	}
	if envelope.EventType != "device.event" || envelope.ProjectID != event.ProjectID {
		return errors.New("DJI_EVENT_SCOPE_INVALID")
	}
	if err := envelope.ValidateForScope(event.ProjectID, envelope.AdapterID); err != nil {
		return err
	}
	var payload canonicalPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return err
	}
	if payload.Protocol != "dji-cloud-api" || payload.RouteKind != string(RouteEvent) {
		return errors.New("DJI_EVENT_PROTOCOL_INVALID")
	}
	if payload.Method != "flighttask_progress" {
		return nil
	}
	progress, err := DecodeWaylineProgress(payload.Data)
	if err != nil {
		return err
	}
	var runID int
	var runStepID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		select command.task_run_id,command.task_run_step_id
		from device_commands command
		join devices device on device.id=command.device_id and device.project_id=command.project_id
		where command.project_id=$1 and device.adapter_id=$2 and command.task_run_id is not null
		  and command.capability_code='mission.execute'
		  and command.parameters_json->>'flight_id'=$3
		order by command.created_at desc limit 1`,
		event.ProjectID, envelope.AdapterID, progress.FlightID).Scan(&runID, &runStepID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	progressJSON, err := json.Marshal(progress)
	if err != nil {
		return err
	}
	if runStepID.Valid {
		if _, err := tx.ExecContext(ctx, `update task_run_steps
			set result_json=jsonb_set(result_json,'{djiWaylineProgress}',$3::jsonb,true)
			where project_id=$1 and id=$2`, event.ProjectID, runStepID.Int64, progressJSON); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `update task_runs
		set output_snapshot_json=jsonb_set(output_snapshot_json,'{djiWaylineProgress}',$3::jsonb,true),
		    state_reason=case when status in ('dispatching','running') then 'dji_wayline_'||$4 else state_reason end
		where project_id=$1 and id=$2`, event.ProjectID, runID, progressJSON, progress.Status)
	return err
}
