package inspection

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aerosight/server/internal/mission"
	"aerosight/server/internal/outbox"
	"github.com/google/uuid"
)

// AssetReader checks actual content, rather than treating a catalogue row marked
// available as proof of readability. The runtime supplies its existing store.
type AssetReader func(context.Context, string) ([]byte, error)
type ObserveInput struct {
	Mode                string  `json:"mode"`
	AssetIDs            []int64 `json:"assetIds"`
	MaxImages           int     `json:"maxImages"`
	ScopeDescription    string  `json:"scopeDescription"`
	ConnectorID         int64   `json:"connectorId"`
	FlightUUID          string  `json:"flightUuid"`
	ConfirmLimitedScope bool    `json:"confirmLimitedScope"`
}

type FlightObservationReader func(context.Context, *sql.Tx, mission.PreparedStep, ObserveInput) (Observation, error)

type RemoteAssetReader func(context.Context, *sql.Tx, mission.PreparedStep, AssetRef) (string, *FlightRef, error)

type ObserveProcessor struct {
	remote RemoteAssetReader
	flight FlightObservationReader
	read   AssetReader
	now    func() time.Time
}

func NewObserveProcessor(read AssetReader, flight ...FlightObservationReader) *ObserveProcessor {
	p := &ObserveProcessor{read: read, now: time.Now}
	if len(flight) > 0 {
		p.flight = flight[0]
	}
	return p
}

func (p *ObserveProcessor) WithRemoteAssetReader(reader RemoteAssetReader) *ObserveProcessor {
	p.remote = reader
	return p
}

func (p *ObserveProcessor) Handler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	step, ok := mission.StepExecution(ctx)
	if !ok {
		return errors.New("INSPECTION_PREPARED_STEP_REQUIRED")
	}
	var input ObserveInput
	raw, err := json.Marshal(step.Parameters)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &input); err != nil {
		return errors.New("INSPECTION_OBSERVE_INPUT_INVALID")
	}
	if input.Mode == "existing-flight" && p.flight != nil {
		observation, err := p.flight(ctx, tx, step, input)
		if err != nil {
			return err
		}
		if observation.Run.Scope != (Scope{ProjectID: step.ProjectID, TeamID: step.TeamID}) || observation.Run.RunID != int64(step.RunID) || observation.Run.StepID != step.StepID {
			return errors.New("INSPECTION_FLIGHT_SCOPE_INVALID")
		}
		return p.seal(ctx, tx, step, observation)
	}
	if input.Mode != "assets" {
		return errors.New("INSPECTION_OBSERVE_MODE_NOT_DEPLOYED")
	}
	if input.MaxImages == 0 {
		input.MaxImages = 64
	}
	if len(input.AssetIDs) == 0 || input.MaxImages < 1 || input.MaxImages > 1000 || len(input.AssetIDs) > input.MaxImages {
		return errors.New("INSPECTION_ASSET_LIMIT_INVALID")
	}
	if p.read == nil && p.remote == nil {
		return errors.New("INSPECTION_ASSET_READER_UNAVAILABLE")
	}
	// Acquire connector locks before asset locks, matching the projector's order.
	idsJSON, _ := json.Marshal(input.AssetIDs)
	rows, err := tx.QueryContext(ctx, `select distinct connector_instance_id from connector_asset_access_refs where project_id=$1 and team_id=$2 and id in(select value::bigint from jsonb_array_elements_text($3)) order by connector_instance_id`, step.ProjectID, step.TeamID, idsJSON)
	if err != nil {
		return err
	}
	var connectors []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		connectors = append(connectors, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range connectors {
		if _, err = LockAlertConnector(ctx, tx, step.ProjectID, id); err != nil {
			return err
		}
	}
	scope := Scope{ProjectID: step.ProjectID, TeamID: step.TeamID}
	observation := Observation{ID: uuid.NewString(), ContractVersion: ContractVersion, Run: RunRef{Scope: scope, RunID: int64(step.RunID), StepID: step.StepID}, Mode: Assets, Completeness: Complete, ScopeDescription: input.ScopeDescription, Assets: []AssetRef{}}
	if observation.ScopeDescription == "" {
		observation.ScopeDescription = "用户明确选择的图片集合"
	}
	seen := map[int64]bool{}
	for _, id := range input.AssetIDs {
		if id < 1 || id > 2147483647 || seen[id] {
			return errors.New("INSPECTION_ASSET_SELECTION_INVALID")
		}
		seen[id] = true
		var version int
		var sourceRun sql.NullInt64
		var key, checksum, objectVersion string
		var remote, simulated bool
		var observed time.Time
		err = tx.QueryRowContext(ctx, `select version,task_run_id,storage_key,coalesce(checksum_sha256,''),coalesce(object_version,''),exists(select 1 from connector_asset_access_refs r where r.project_id=assets.project_id and r.id=assets.id),coalesce(captured_at at time zone 'UTC',available_at,created_at at time zone 'UTC'),exists(select 1 from devices d join device_adapters adapter on adapter.id=d.adapter_id and adapter.project_id=d.project_id where d.id=assets.device_id and d.project_id=assets.project_id and adapter.team_id=assets.team_id and adapter.adapter_type='simulator') from assets
    where project_id=$1 and team_id=$2 and id=$3 and kind='image' and status='available' for share`, step.ProjectID, step.TeamID, id).Scan(&version, &sourceRun, &key, &checksum, &objectVersion, &remote, &observed, &simulated)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("INSPECTION_ASSET_SCOPE_OR_STATE_INVALID")
		}
		if err != nil {
			return err
		}
		asset := AssetRef{Scope: scope, AssetID: id, Version: version, ObjectVersion: objectVersion, SourceDeviceMode: "unverified"}
		if simulated {
			asset.SourceDeviceMode = "simulator"
		}
		if sourceRun.Valid {
			v := sourceRun.Int64
			asset.SourceRunID = &v
		}
		var actual string
		if remote {
			if p.remote == nil {
				return errors.New("INSPECTION_REMOTE_ASSET_READER_UNAVAILABLE")
			}
			actual, asset.Flight, err = p.remote(ctx, tx, step, asset)
			if err != nil {
				return err
			}
		} else {
			if p.read == nil {
				return errors.New("INSPECTION_ASSET_READER_UNAVAILABLE")
			}
			body, readErr := p.read(ctx, key)
			if readErr != nil || len(body) == 0 {
				return errors.New("INSPECTION_ASSET_UNREADABLE")
			}
			digest := sha256.Sum256(body)
			actual = hex.EncodeToString(digest[:])
		}
		if !validChecksum(actual) {
			return errors.New("INSPECTION_ASSET_CHECKSUM_INVALID")
		}
		if checksum != "" && checksum != actual {
			return errors.New("INSPECTION_ASSET_CHECKSUM_MISMATCH")
		}
		asset.ChecksumSHA256 = actual
		observation.Assets = append(observation.Assets, asset)
		if observation.ObservedFrom.IsZero() || observed.Before(observation.ObservedFrom) {
			observation.ObservedFrom = observed
		}
		if observation.ObservedTo.IsZero() || observed.After(observation.ObservedTo) {
			observation.ObservedTo = observed
		}
	}
	return p.seal(ctx, tx, step, observation)
}

func (p *ObserveProcessor) seal(ctx context.Context, tx *sql.Tx, step mission.PreparedStep, observation Observation) error {
	var err error
	if err = observation.Validate(); err != nil {
		return err
	}
	manifest, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	var connectorID, flightID, projectedRun any
	if observation.Flight != nil {
		connectorID = observation.Flight.ConnectorID
		flightID = observation.Flight.FlightUUID
		projectedRun = observation.Flight.ProjectedRunID
	}
	if _, err = tx.ExecContext(ctx, `insert into inspection_observations(id,project_id,team_id,task_run_id,task_run_step_id,source_mode,completeness,scope_description,observed_from,observed_to,manifest_json,sealed_at,connector_instance_id,remote_flight_id,projected_run_id,limited_scope_confirmed_by)
 values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, observation.ID, step.ProjectID, step.TeamID, step.RunID, step.StepID, observation.Mode, observation.Completeness, observation.ScopeDescription, observation.ObservedFrom, observation.ObservedTo, manifest, p.now().UTC(), connectorID, flightID, projectedRun, observation.LimitedScopeConfirmedBy); err != nil {
		return err
	}
	for _, asset := range observation.Assets {
		if _, err = tx.ExecContext(ctx, `insert into inspection_observation_assets(observation_id,project_id,asset_id,asset_version,source_run_id) values($1,$2,$3,$4,$5)`, observation.ID, step.ProjectID, asset.AssetID, asset.Version, asset.SourceRunID); err != nil {
			return err
		}
	}
	output := map[string]any{"observationId": observation.ID}
	if _, err = tx.ExecContext(ctx, "update task_run_steps set status='succeeded',output_snapshot_json=$3,result_json=result_json||$3,finished_at=$4 where project_id=$1 and id=$2", step.ProjectID, step.StepID, output, p.now().UTC()); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json) values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, step.ProjectID, step.TeamID, fmt.Sprintf("inspection-observe:%d:succeeded", step.StepID), map[string]any{"taskRunId": step.RunID, "to": "running", "completedStepId": step.StepID})
	return err
}
