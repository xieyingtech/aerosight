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
}

type Result struct {
	OutputReferences []map[string]string `json:"outputReferences"`
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
	recorder := sqlRunRecorder{tx: tx, projectID: event.ProjectID}
	return Process(ctx, Request{RunID: payload.AutomationRunID, PerceptionEventID: payload.PerceptionEventID,
		ProjectID: event.ProjectID, TeamID: event.TeamID}, processor.Generator, recorder, time.Now().UTC())
}

type sqlRunRecorder struct {
	tx        *sql.Tx
	projectID int
}

func (recorder sqlRunRecorder) Succeeded(ctx context.Context, runID string, result Result, now time.Time) error {
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
