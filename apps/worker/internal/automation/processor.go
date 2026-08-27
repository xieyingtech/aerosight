package automation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"aerosight/worker/internal/outbox"
)

var ErrModelUnavailable = errors.New("model unavailable")

type Request struct {
	RunID, PerceptionEventID string
	ProjectID, TeamID        int
	Mode                     string
}

type Result struct {
	OutputReferences []map[string]string `json:"outputReferences"`
	Drafts           []Draft             `json:"drafts"`
}

type Generator interface {
	Generate(context.Context, Request) (Result, error)
}

type RunRecorder interface {
	Succeeded(context.Context, string, Result, time.Time) error
	Failed(context.Context, string, error, time.Time) error
}

func Process(ctx context.Context, request Request, generator Generator, recorder RunRecorder, now time.Time) error {
	result, err := generator.Generate(ctx, request)
	if err != nil {
		return recorder.Failed(ctx, request.RunID, err, now)
	}
	return recorder.Succeeded(ctx, request.RunID, result, now)
}

type Processor struct{ Generator Generator }

func (processor Processor) Handler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var payload struct {
		AutomationRunID, PerceptionEventID string
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	if payload.AutomationRunID == "" || payload.PerceptionEventID == "" {
		return errors.New("alert automation event is missing identifiers")
	}
	var mode string
	var enabled bool
	if err := tx.QueryRowContext(ctx, `select version.mode,flags.automatic_ai_enabled from alert_automation_runs run
		join alert_automation_policy_versions version on version.id=run.policy_version_id and version.project_id=run.project_id
		join project_feature_flags flags on flags.project_id=run.project_id
		where run.id=$1 and run.project_id=$2`, payload.AutomationRunID, event.ProjectID).Scan(&mode, &enabled); err != nil {
		return err
	}
	if !ShouldContinueAutomation(enabled) {
		_, err := tx.ExecContext(ctx, `update alert_automation_runs set status='canceled',failure_code='AUTOMATIC_AI_DISABLED',finished_at=now() where id=$1 and project_id=$2`, payload.AutomationRunID, event.ProjectID)
		return err
	}
	recorder := sqlRunRecorder{tx: tx, projectID: event.ProjectID}
	return Process(ctx, Request{RunID: payload.AutomationRunID, PerceptionEventID: payload.PerceptionEventID,
		ProjectID: event.ProjectID, TeamID: event.TeamID, Mode: mode}, processor.Generator, recorder, time.Now().UTC())
}

type sqlRunRecorder struct {
	tx        *sql.Tx
	projectID int
}

func (recorder sqlRunRecorder) Succeeded(ctx context.Context, runID string, result Result, now time.Time) error {
	var enabled bool
	if err := recorder.tx.QueryRowContext(ctx, `select coalesce(flags.automatic_ai_enabled,false) from alert_automation_runs run left join project_feature_flags flags on flags.project_id=run.project_id where run.id=$1 and run.project_id=$2`, runID, recorder.projectID).Scan(&enabled); err != nil {
		return err
	}
	if !ShouldContinueAutomation(enabled) {
		_, err := recorder.tx.ExecContext(ctx, `update alert_automation_runs set status='canceled',failure_code='AUTOMATIC_AI_DISABLED',finished_at=$3 where id=$1 and project_id=$2`, runID, recorder.projectID, now)
		return err
	}
	for _, draft := range result.Drafts {
		if draft.Status != "draft" || draft.ExternalEffect != "none" {
			return errors.New("automation output violated draft-only boundary")
		}
		payload, err := json.Marshal(draft.Payload)
		if err != nil {
			return err
		}
		evidence, err := json.Marshal(draft.Evidence)
		if err != nil {
			return err
		}
		var draftID string
		if err := recorder.tx.QueryRowContext(ctx, `insert into alert_automation_drafts(project_id,team_id,automation_run_id,perception_event_id,draft_type,status,title,payload_json,evidence_refs_json)
			select run.project_id,run.team_id,run.id,run.perception_event_id,$3,'draft',$4,$5,$6 from alert_automation_runs run where run.id=$1 and run.project_id=$2
			on conflict(automation_run_id,draft_type) do update set title=excluded.title returning id`, runID, recorder.projectID, draft.Type, draft.Title, payload, evidence).Scan(&draftID); err != nil {
			return err
		}
		result.OutputReferences = append(result.OutputReferences, map[string]string{"type": draft.Type, "id": draftID, "status": "draft"})
	}
	output, err := json.Marshal(result.OutputReferences)
	if err != nil {
		return err
	}
	_, err = recorder.tx.ExecContext(ctx, `update alert_automation_runs set status='succeeded',output_refs_json=$3,finished_at=$4
		where id=$1 and project_id=$2`, runID, recorder.projectID, output, now)
	return err
}

func (recorder sqlRunRecorder) Failed(ctx context.Context, runID string, cause error, now time.Time) error {
	_, err := recorder.tx.ExecContext(ctx, `update alert_automation_runs set status='failed',failure_code='MODEL_GENERATION_FAILED',
		failure_message=left($3,2000),finished_at=$4 where id=$1 and project_id=$2`, runID, recorder.projectID, cause.Error(), now)
	return err
}

type UnavailableGenerator struct{}

func (UnavailableGenerator) Generate(context.Context, Request) (Result, error) {
	return Result{}, ErrModelUnavailable
}
