package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/mission"
	"github.com/google/uuid"
)

type ExternalEvidence struct {
	Ref                 string                    `json:"ref"`
	Asset               AssetRef                  `json:"asset"`
	AlgorithmRunID      string                    `json:"algorithmRunId"`
	DefinitionVersionID int64                     `json:"definitionVersionId"`
	ModelRevision       string                    `json:"modelRevision"`
	ModelDigest         string                    `json:"modelDigest"`
	Result              algorithm.CanonicalResult `json:"result"`
}

func (p *DetectProcessor) external(ctx context.Context, tx *sql.Tx, step mission.PreparedStep) error {
	if p.trigger == nil {
		return errors.New("INSPECTION_EXTERNAL_DETECT_NOT_DEPLOYED")
	}
	var input struct {
		ObservationID string         `json:"observationId"`
		DefinitionID  int64          `json:"algorithmDefinitionVersionId"`
		MaxImages     int            `json:"maxImages"`
		Parameters    map[string]any `json:"parameters"`
	}
	raw, err := json.Marshal(step.Parameters)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &input); err != nil || input.DefinitionID <= 0 {
		return errors.New("INSPECTION_EXTERNAL_INPUT_INVALID")
	}
	if _, err = uuid.Parse(input.ObservationID); err != nil {
		return errors.New("INSPECTION_OBSERVATION_SCOPE_INVALID")
	}
	if input.MaxImages == 0 {
		input.MaxImages = 64
	}
	var manifest []byte
	err = tx.QueryRowContext(ctx, `select manifest_json from inspection_observations where project_id=$1 and team_id=$2 and task_run_id=$3 and id=$4 and sealed_at is not null`, step.ProjectID, step.TeamID, step.RunID, input.ObservationID).Scan(&manifest)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("INSPECTION_OBSERVATION_SCOPE_INVALID")
	}
	if err != nil {
		return err
	}
	var observation Observation
	if err = json.Unmarshal(manifest, &observation); err != nil {
		return err
	}
	if err = observation.Validate(); err != nil {
		return err
	}
	if input.MaxImages < 1 || input.MaxImages > 1000 || len(observation.Assets) == 0 || len(observation.Assets) > input.MaxImages {
		return errors.New("INSPECTION_EXTERNAL_IMAGE_LIMIT_INVALID")
	}
	var count int
	if err = tx.QueryRowContext(ctx, `select count(*) from algorithm_runs where project_id=$1 and task_run_id=$2 and task_run_step_id=$3`, step.ProjectID, step.RunID, step.StepID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		for _, asset := range observation.Assets {
			if err = p.trigger.QueueInspectionAsset(ctx, tx, step.ProjectID, step.TeamID, step.RunID, step.StepID, observation.ID, int(asset.AssetID), input.DefinitionID, input.Parameters); err != nil {
				return err
			}
		}
		return nil
	}
	if count != len(observation.Assets) {
		return errors.New("INSPECTION_EXTERNAL_CHILD_SET_INVALID")
	}
	expected := map[int64]AssetRef{}
	for _, asset := range observation.Assets {
		expected[asset.AssetID] = asset
	}
	rows, err := tx.QueryContext(ctx, `select id::text,input_asset_id,algorithm_definition_version_id,status,canonical_result_json from algorithm_runs where project_id=$1 and task_run_id=$2 and task_run_step_id=$3 order by input_asset_id,id`, step.ProjectID, step.RunID, step.StepID)
	if err != nil {
		return err
	}
	defer rows.Close()
	evidence := EvidenceSet{ID: uuid.NewString(), Run: RunRef{Scope: Scope{ProjectID: step.ProjectID, TeamID: step.TeamID}, RunID: int64(step.RunID), StepID: step.StepID}, ObservationID: observation.ID, Source: "external", ModelVersion: "unknown", Completeness: Complete, TargetAlgorithmConfirmed: true, EvidenceRefs: []string{"observation:" + observation.ID}, Candidates: []Candidate{}, ExternalResults: []ExternalEvidence{}}
	pending, failed := false, false
	for rows.Next() {
		var runID, state string
		var assetID, definitionID int64
		var canonical []byte
		if err = rows.Scan(&runID, &assetID, &definitionID, &state, &canonical); err != nil {
			return err
		}
		asset, ok := expected[assetID]
		if !ok || definitionID != input.DefinitionID {
			return errors.New("INSPECTION_EXTERNAL_CHILD_SET_INVALID")
		}
		delete(expected, assetID)
		switch state {
		case "queued", "running", "polling", "waiting_callback":
			pending = true
			continue
		case "succeeded":
		case "failed", "canceled", "timed_out":
			failed = true
			continue
		default:
			return errors.New("INSPECTION_EXTERNAL_CHILD_STATE_INVALID")
		}
		var result struct {
			Result algorithm.CanonicalResult `json:"result"`
			Source struct {
				ModelRevision string `json:"modelRevision"`
				ModelDigest   string `json:"modelDigest"`
			} `json:"source"`
		}
		if err = json.Unmarshal(canonical, &result); err != nil || result.Result.Validate() != nil || result.Result.Kind != algorithm.ResultDetection {
			return errors.New("INSPECTION_EXTERNAL_RESULT_INVALID")
		}
		ref := "algorithm:" + runID
		revision := result.Source.ModelRevision
		if revision == "" {
			revision = "unknown"
		}
		digest := result.Source.ModelDigest
		if digest == "" {
			digest = "unknown"
		}
		evidence.EvidenceRefs = append(evidence.EvidenceRefs, ref)
		evidence.ExternalResults = append(evidence.ExternalResults, ExternalEvidence{Ref: ref, Asset: asset, AlgorithmRunID: runID, DefinitionVersionID: definitionID, ModelRevision: revision, ModelDigest: digest, Result: result.Result})
		for n := range result.Result.Detections {
			evidence.Candidates = append(evidence.Candidates, Candidate{ID: fmt.Sprintf("external:%s:%d", runID, n), EvidenceRefs: []string{ref}, Position: Position{Source: "unknown", Quality: "image-only"}})
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if len(expected) != 0 {
		return errors.New("INSPECTION_EXTERNAL_CHILD_SET_INVALID")
	}
	if pending {
		return nil
	}
	if failed {
		return errors.New("INSPECTION_EXTERNAL_CHILD_FAILED")
	}
	// Partial observations can only be complete for a separately confirmed finite
	// set. The original observation retains its full-flight partial status.
	if observation.Completeness != Complete && observation.LimitedScopeConfirmedBy == nil {
		return errors.New("INSPECTION_SCOPE_NOT_CONFIRMED")
	}
	return sealEvidence(ctx, tx, step, observation, evidence)
}
