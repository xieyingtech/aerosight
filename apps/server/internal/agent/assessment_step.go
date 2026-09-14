package agent

import (
	"aerosight/server/internal/inspection"
	"aerosight/server/internal/mission"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
)

const inspectionAssessmentPromptVersion = "inspection-assessment-v2"

// Called inside the mission Run lock. The author never supplies user identity,
// session identity or the evidence payload. Registration waits for the worker.
func queueInspectionAssessment(ctx context.Context, tx *sql.Tx, step mission.PreparedStep) error {
	var input struct {
		Mode          string   `json:"mode"`
		EvidenceSetID string   `json:"evidenceSetId"`
		Temperature   *float64 `json:"temperature"`
	}
	raw, err := json.Marshal(step.Parameters)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &input); err != nil || input.Mode != "assessment" {
		return errors.New("INSPECTION_ASSESSMENT_INPUT_INVALID")
	}
	temperature := 0.2
	if input.Temperature != nil {
		temperature = *input.Temperature
	}
	if !(temperature >= 0 && temperature <= 2) {
		return errors.New("INSPECTION_ASSESSMENT_TEMPERATURE_INVALID")
	}
	if _, err = uuid.Parse(input.EvidenceSetID); err != nil {
		return errors.New("INSPECTION_EVIDENCE_SCOPE_INVALID")
	}
	var evidenceRaw, observationRaw []byte
	err = tx.QueryRowContext(ctx, `select e.evidence_json,o.manifest_json from inspection_evidence_sets e join inspection_observations o on o.id=e.observation_id and o.project_id=e.project_id and o.task_run_id=e.task_run_id where e.id=$1 and e.project_id=$2 and e.team_id=$3 and e.task_run_id=$4`, input.EvidenceSetID, step.ProjectID, step.TeamID, step.RunID).Scan(&evidenceRaw, &observationRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("INSPECTION_EVIDENCE_SCOPE_INVALID")
	}
	if err != nil {
		return err
	}
	var evidence inspection.EvidenceSet
	if err = json.Unmarshal(evidenceRaw, &evidence); err != nil {
		return errors.New("INSPECTION_EVIDENCE_INVALID")
	}
	var observation inspection.Observation
	if err = json.Unmarshal(observationRaw, &observation); err != nil {
		return errors.New("INSPECTION_OBSERVATION_INVALID")
	}
	if err = evidence.Validate(observation); err != nil {
		return err
	}
	if evidence.ID != input.EvidenceSetID || evidence.Run.RunID != int64(step.RunID) || evidence.Run.ProjectID != step.ProjectID || evidence.Run.TeamID != step.TeamID {
		return errors.New("INSPECTION_EVIDENCE_SCOPE_INVALID")
	}
	var existing string
	err = tx.QueryRowContext(ctx, `select id::text from inspection_assessments where project_id=$1 and task_run_step_id=$2`, step.ProjectID, step.StepID).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var copilotID int
	err = tx.QueryRowContext(ctx, `select id from agents where project_id=$1 and status='active' and config_json->>'kind'='copilot' order by id limit 1`, step.ProjectID).Scan(&copilotID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("INSPECTION_COPILOT_UNAVAILABLE")
	}
	if err != nil {
		return err
	}
	assessmentID := uuid.NewString()
	digest := sha256.Sum256(evidenceRaw)
	if _, err = tx.ExecContext(ctx, `insert into inspection_assessments(id,project_id,team_id,task_run_id,task_run_step_id,evidence_set_id,prompt_version,evidence_hash) values($1,$2,$3,$4,$5,$6,$7,$8)`, assessmentID, step.ProjectID, step.TeamID, step.RunID, step.StepID, input.EvidenceSetID, inspectionAssessmentPromptVersion, hex.EncodeToString(digest[:])); err != nil {
		return err
	}
	var sessionID int
	if err = tx.QueryRowContext(ctx, `insert into agent_sessions(project_id,agent_id,task_run_id,started_by_user_id,summary) values($1,$2,$3,$4,$5) returning id`, step.ProjectID, copilotID, step.RunID, step.UserID, fmt.Sprintf("巡检研判 · Task Run #%d", step.RunID)).Scan(&sessionID); err != nil {
		return err
	}
	args := map[string]any{"assessmentId": assessmentID, "evidenceSetId": input.EvidenceSetID, "taskRunId": step.RunID, "taskRunStepId": step.StepID, "promptVersion": inspectionAssessmentPromptVersion, "temperature": temperature}
	_, err = tx.ExecContext(ctx, `insert into agent_tool_jobs(project_id,team_id,session_id,requested_by_user_id,trigger_type,idempotency_key,tool_name,required_permission,args_json,context_expires_at) values($1,$2,$3,$4,'task_step',$5,'inspection_assessment','agent:use',$6,now()+interval '24 hours')`, step.ProjectID, step.TeamID, sessionID, step.UserID, fmt.Sprintf("inspection-assessment:%d", step.StepID), args)
	return err
}
