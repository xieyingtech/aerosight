package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/outbox"
	"github.com/google/uuid"
)

// NativeAlertEvidence is frozen at detection time. Later connector updates must
// not change the evidence which an assessment consumed. Target values retain
// provider semantics and are deliberately not named confidence.
type NativeAlertEvidence struct {
	Ref              string          `json:"ref"`
	AlgorithmSource  *int            `json:"algorithmSource"`
	ModelVersion     string          `json:"modelVersion"`
	Timestamp        json.RawMessage `json:"timestamp,omitempty"`
	Reason           string          `json:"reason"`
	Status           json.RawMessage `json:"status,omitempty"`
	Targets          json.RawMessage `json:"targets"`
	Position         Position        `json:"position"`
	ProviderLocation json.RawMessage `json:"providerLocation,omitempty"`
	SourceAvailable  bool            `json:"sourceAvailable"`
}

type DetectProcessor struct{ trigger *algorithm.Trigger }

func NewDetectProcessor(trigger ...*algorithm.Trigger) *DetectProcessor {
	p := &DetectProcessor{}
	if len(trigger) > 0 {
		p.trigger = trigger[0]
	}
	return p
}

func (p *DetectProcessor) Handler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	step, ok := mission.StepExecution(ctx)
	if !ok {
		return errors.New("INSPECTION_PREPARED_STEP_REQUIRED")
	}
	source, _ := step.Parameters["source"].(string)
	if source == "external" {
		return p.external(ctx, tx, step)
	}
	if source != "flighthub-ai" {
		return errors.New("INSPECTION_DETECT_SOURCE_NOT_DEPLOYED")
	}
	observationID, _ := step.Parameters["observationId"].(string)
	if _, err := uuid.Parse(observationID); err != nil {
		return errors.New("INSPECTION_OBSERVATION_SCOPE_INVALID")
	}
	var raw []byte
	err := tx.QueryRowContext(ctx, `select manifest_json from inspection_observations where id=$1 and project_id=$2 and team_id=$3 and task_run_id=$4 and sealed_at is not null`, observationID, step.ProjectID, step.TeamID, step.RunID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("INSPECTION_OBSERVATION_SCOPE_INVALID")
	}
	if err != nil {
		return err
	}
	var observation Observation
	if err = json.Unmarshal(raw, &observation); err != nil {
		return err
	}
	if err = observation.Validate(); err != nil {
		return err
	}
	if observation.Flight == nil {
		return errors.New("INSPECTION_DETECTION_SOURCE_INVALID")
	}
	// Serialize against alert projection. This both freezes a coherent snapshot
	// and prevents native alerts from bypassing the Task assessment after binding.
	if err = ClaimAlertFlight(ctx, tx, step.ProjectID, observation.Flight.ConnectorID, observation.Flight.FlightUUID, int64(step.RunID)); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `select source.remote_resource_id,source.evidence_json,resource.status
 from inspection_alert_sources source join connector_remote_resources resource
 on resource.id=source.remote_resource_id and resource.project_id=source.project_id and resource.connector_instance_id=source.connector_instance_id
 where source.project_id=$1 and source.connector_instance_id=$2 and source.remote_flight_id=$3
 order by source.remote_resource_id limit 1001`, step.ProjectID, observation.Flight.ConnectorID, observation.Flight.FlightUUID)
	if err != nil {
		return err
	}
	defer rows.Close()
	scopeRef := "observation:" + observation.ID
	evidence := EvidenceSet{ID: uuid.NewString(), Run: RunRef{Scope: Scope{ProjectID: step.ProjectID, TeamID: step.TeamID}, RunID: int64(step.RunID), StepID: step.StepID}, ObservationID: observation.ID, Source: source, ModelVersion: "unknown", Completeness: Unavailable, EvidenceRefs: []string{scopeRef}, Candidates: []Candidate{}, NativeAlerts: []NativeAlertEvidence{}, DataGaps: []string{"native_algorithm_enabled_unknown", "native_analysis_scope_unknown", "native_model_version_unknown"}}
	for rows.Next() {
		if len(evidence.NativeAlerts) >= 1000 {
			return errors.New("INSPECTION_NATIVE_ALERT_LIMIT_EXCEEDED")
		}
		var resourceID int64
		var stored []byte
		var status string
		if err = rows.Scan(&resourceID, &stored, &status); err != nil {
			return err
		}
		var alert struct {
			AlgorithmSource *int            `json:"algorithmSource"`
			Timestamp       json.RawMessage `json:"timestamp"`
			Reason          string          `json:"reason"`
			Status          json.RawMessage `json:"status"`
			Targets         json.RawMessage `json:"targets"`
			Location        json.RawMessage `json:"location"`
		}
		if err = json.Unmarshal(stored, &alert); err != nil || len(alert.Targets) == 0 {
			return errors.New("INSPECTION_NATIVE_ALERT_INVALID")
		}
		ref := fmt.Sprintf("flighthub-alert:%d:%d", step.ProjectID, resourceID)
		position := Position{Source: "unknown", Quality: "unverified"}
		item := NativeAlertEvidence{Ref: ref, AlgorithmSource: alert.AlgorithmSource, ModelVersion: "unknown", Timestamp: alert.Timestamp, Reason: alert.Reason, Status: alert.Status, Targets: alert.Targets, ProviderLocation: alert.Location, Position: position, SourceAvailable: status == "active"}
		evidence.NativeAlerts = append(evidence.NativeAlerts, item)
		evidence.EvidenceRefs = append(evidence.EvidenceRefs, ref)
		if status == "active" {
			evidence.Candidates = append(evidence.Candidates, Candidate{ID: ref, EvidenceRefs: []string{ref}, Position: position})
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if len(evidence.NativeAlerts) > 0 {
		evidence.Completeness = AlertOnly
	}
	for _, alert := range evidence.NativeAlerts {
		if !alert.SourceAvailable {
			evidence.Completeness = Partial
			evidence.DataGaps = append(evidence.DataGaps, "native_alert_source_unavailable")
			break
		}
	}
	return sealEvidence(ctx, tx, step, observation, evidence)
}

func sealEvidence(ctx context.Context, tx *sql.Tx, step mission.PreparedStep, observation Observation, evidence EvidenceSet) error {
	var err error
	if err = evidence.Validate(observation); err != nil {
		return err
	}
	frozen, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `insert into inspection_evidence_sets(id,project_id,team_id,task_run_id,task_run_step_id,observation_id,source,model_version,completeness,target_algorithm_confirmed,evidence_json)
 values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, evidence.ID, step.ProjectID, step.TeamID, step.RunID, step.StepID, observation.ID, evidence.Source, evidence.ModelVersion, evidence.Completeness, evidence.TargetAlgorithmConfirmed, frozen)
	if err != nil {
		return err
	}
	output := map[string]any{"evidenceSetId": evidence.ID}
	now := time.Now().UTC()
	if _, err = tx.ExecContext(ctx, `update task_run_steps set status='succeeded',output_snapshot_json=$3,result_json=result_json||$3,finished_at=$4 where project_id=$1 and id=$2`, step.ProjectID, step.StepID, output, now); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json) values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, step.ProjectID, step.TeamID, fmt.Sprintf("inspection-detect:%d:succeeded", step.StepID), map[string]any{"taskRunId": step.RunID, "to": "running", "completedStepId": step.StepID})
	return err
}
