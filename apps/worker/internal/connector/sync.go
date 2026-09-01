package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var ErrSyncCursorAdvanced = errors.New("connector sync cursor advanced concurrently")

type SyncApplyResult struct {
	RunID          int64
	Discovered     int
	Managed        int
	Missing        int
	ReplayedCursor bool
}

type SyncStore interface {
	CurrentCursor(context.Context, Instance) (json.RawMessage, error)
	ApplyBatch(context.Context, Instance, DiscoveryMode, json.RawMessage, DiscoveryBatch) (SyncApplyResult, error)
}

type Synchronizer struct {
	registry *Registry
	store    SyncStore
}

func NewSynchronizer(registry *Registry, store SyncStore) (*Synchronizer, error) {
	if registry == nil || store == nil {
		return nil, errors.New("connector registry and sync store are required")
	}
	return &Synchronizer{registry: registry, store: store}, nil
}

func (synchronizer *Synchronizer) Run(ctx context.Context, instance Instance, mode DiscoveryMode) (SyncApplyResult, error) {
	if instance.ID <= 0 || instance.ProjectID <= 0 || instance.ConnectorKey == "" || instance.Version == "" {
		return SyncApplyResult{}, errors.New("connector instance identity is invalid")
	}
	runtime, err := synchronizer.registry.Resolve(instance.ConnectorKey, instance.Version)
	if err != nil {
		return SyncApplyResult{}, err
	}
	handler := runtime.DiscoveryHandlers[mode]
	if handler == nil {
		return SyncApplyResult{}, fmt.Errorf("connector does not support discovery mode %q", mode)
	}
	cursor, err := synchronizer.store.CurrentCursor(ctx, instance)
	if err != nil {
		return SyncApplyResult{}, err
	}
	batch, err := handler(ctx, DiscoveryRequest{Instance: instance, Mode: mode, Cursor: cursor})
	if err != nil {
		return SyncApplyResult{}, err
	}
	seen := make(map[string]struct{}, len(batch.Devices))
	unique := make([]ExternalDevice, 0, len(batch.Devices))
	for _, device := range batch.Devices {
		if device.ExternalID == "" {
			return SyncApplyResult{}, errors.New("discovered external device identity is required")
		}
		if !runtime.ScopeFilter(instance, device) {
			return SyncApplyResult{}, fmt.Errorf("external device %q is outside connector discovery scope", device.ExternalID)
		}
		if _, duplicate := seen[device.ExternalID]; duplicate {
			continue
		}
		seen[device.ExternalID] = struct{}{}
		unique = append(unique, device)
	}
	batch.Devices = unique
	return synchronizer.store.ApplyBatch(ctx, instance, mode, cursor, batch)
}

type SQLSyncStore struct{ db *sql.DB }

func NewSQLSyncStore(db *sql.DB) *SQLSyncStore { return &SQLSyncStore{db: db} }

func (store *SQLSyncStore) CurrentCursor(ctx context.Context, instance Instance) (json.RawMessage, error) {
	var cursor []byte
	err := store.db.QueryRowContext(ctx, `
		select adapter.sync_cursor_json
		  from device_adapters adapter
		  join connector_definitions definition on definition.id=adapter.connector_definition_id
		 where adapter.id=$1 and adapter.project_id=$2
		   and definition.connector_key=$3 and definition.version=$4
		   and ($5='' or (adapter.lease_owner=$5 and adapter.connection_epoch=$6 and adapter.lease_expires_at>=now()))`,
		instance.ID, instance.ProjectID, instance.ConnectorKey, instance.Version,
		instance.LeaseOwner, instance.LeaseEpoch).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("connector instance is unavailable or out of scope")
	}
	return json.RawMessage(cursor), err
}

func sameJSON(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func (store *SQLSyncStore) ApplyBatch(
	ctx context.Context, instance Instance, mode DiscoveryMode, expectedCursor json.RawMessage, batch DiscoveryBatch,
) (result SyncApplyResult, returnedErr error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	var teamID int
	var onboardingPolicy string
	var currentCursor, scope []byte
	err = tx.QueryRowContext(ctx, `
		select team_id, sync_cursor_json, discovery_scope_json, onboarding_policy
		  from device_adapters where id=$1 and project_id=$2
		   and ($3='' or (lease_owner=$3 and connection_epoch=$4 and lease_expires_at>=now()))
		 for update`,
		instance.ID, instance.ProjectID, instance.LeaseOwner, instance.LeaseEpoch).Scan(&teamID, &currentCursor, &scope, &onboardingPolicy)
	if errors.Is(err, sql.ErrNoRows) {
		return result, errors.New("connector instance is unavailable or out of scope")
	}
	if err != nil {
		return result, err
	}
	if !sameJSON(currentCursor, expectedCursor) {
		return result, ErrSyncCursorAdvanced
	}
	err = tx.QueryRowContext(ctx, `
		insert into connector_sync_runs (
		  project_id,team_id,connector_instance_id,discovery_mode,status,scope_json,cursor_before_json,started_at
		) values ($1,$2,$3,$4,'running',$5,$6,now()) returning id`,
		instance.ProjectID, teamID, instance.ID, mode, scope, currentCursor).Scan(&result.RunID)
	if err != nil {
		return result, err
	}
	if sameJSON(currentCursor, batch.Cursor) {
		result.ReplayedCursor = true
	} else {
		for _, device := range batch.Devices {
			identity, marshalErr := json.Marshal(map[string]any{
				"attributes": device.Attributes, "parentExternalId": device.ParentExternalID,
			})
			if marshalErr != nil {
				return result, marshalErr
			}
			_, err = tx.ExecContext(ctx, `
				insert into device_external_identities (
				  project_id,team_id,adapter_id,external_device_id,external_device_type,suggested_device_type_id,identity_json,
				  match_confidence,discovery_status,source_version,last_sync_run_id,first_seen_at,last_seen_at
				) values ($1,$2,$3,$4,$5,
				  (select id from device_types where type_key=$5 and status='active' order by version desc limit 1),
				  $6,case when exists(select 1 from device_types where type_key=$5 and status='active') then 1 else null end,
				  'discovered',$7,$8,now(),now())
				on conflict (adapter_id,external_device_id) do update set
				  external_device_type=excluded.external_device_type,
				  suggested_device_type_id=excluded.suggested_device_type_id,
				  match_confidence=excluded.match_confidence,
				  identity_json=excluded.identity_json,
				  discovery_status=case
				    when device_external_identities.discovery_status in ('managed','ignored','conflicted')
				      then device_external_identities.discovery_status else 'discovered' end,
				  source_version=excluded.source_version,last_sync_run_id=excluded.last_sync_run_id,last_seen_at=now()`,
				instance.ProjectID, teamID, instance.ID, device.ExternalID, device.ExternalType,
				identity, batch.SourceVersion, result.RunID)
			if err != nil {
				return result, err
			}
			serial, hasSerial := device.Attributes["serialNumber"].(string)
			serial = strings.TrimSpace(serial)
			if hasSerial && serial != "" {
				conflict, conflictErr := tx.ExecContext(ctx, `
					update device_external_identities identity
					   set discovery_status='conflicted'
					 where identity.project_id=$1 and identity.adapter_id=$2 and identity.external_device_id=$3
					   and identity.discovery_status in ('discovered','managed','conflicted')
					   and exists (
					     select 1 from device_external_identities other
					      where other.project_id=identity.project_id and other.adapter_id<>identity.adapter_id
					        and other.discovery_status in ('discovered','managed','conflicted')
					        and (other.external_device_id=$4 or other.identity_json#>>'{attributes,serialNumber}'=$4)
					   )`, instance.ProjectID, instance.ID, device.ExternalID, serial)
				if conflictErr != nil {
					return result, conflictErr
				}
				conflicted, rowsErr := conflict.RowsAffected()
				if rowsErr != nil {
					return result, rowsErr
				}
				if conflicted > 0 {
					_, err = tx.ExecContext(ctx, `
						update device_external_identities other
						   set discovery_status='conflicted'
						 where other.project_id=$1 and other.adapter_id<>$2
						   and other.discovery_status in ('discovered','managed','conflicted')
						   and (other.external_device_id=$3 or other.identity_json#>>'{attributes,serialNumber}'=$3)`,
						instance.ProjectID, instance.ID, serial)
					if err != nil {
						return result, err
					}
				}
			}
			managed, onboardErr := store.applyAutomaticOnboarding(
				ctx, tx, instance, teamID, OnboardingPolicy(onboardingPolicy), device,
			)
			if onboardErr != nil {
				return result, onboardErr
			}
			if managed {
				result.Managed++
			}
		}
		if onboardingPolicy == string(OnboardingAutomatic) {
			if err = store.materializeAutomaticRelationships(ctx, tx, instance, teamID, batch.Devices); err != nil {
				return result, err
			}
		}
		result.Discovered = len(batch.Devices)
		if batch.CompleteSnapshot {
			seen, marshalErr := json.Marshal(externalIDs(batch.Devices))
			if marshalErr != nil {
				return result, marshalErr
			}
			missing, execErr := tx.ExecContext(ctx, `
				update device_external_identities identity set discovery_status='missing',last_sync_run_id=$3
				 where identity.project_id=$1 and identity.adapter_id=$2
				   and identity.discovery_status in ('discovered','managed')
				   and not exists (
				     select 1 from jsonb_array_elements_text($4::jsonb) seen(id)
				      where seen.id=identity.external_device_id
				   )`, instance.ProjectID, instance.ID, result.RunID, seen)
			if execErr != nil {
				return result, execErr
			}
			count, rowsErr := missing.RowsAffected()
			if rowsErr != nil {
				return result, rowsErr
			}
			result.Missing = int(count)
		}
		_, err = tx.ExecContext(ctx, `update device_adapters set sync_cursor_json=$3,updated_at=now()
			where id=$1 and project_id=$2`, instance.ID, instance.ProjectID, normalizedCursor(batch.Cursor))
		if err != nil {
			return result, err
		}
	}
	_, err = tx.ExecContext(ctx, `
		update connector_sync_runs set status='succeeded',cursor_after_json=$2,
		  discovered_count=$3,managed_count=$4,missing_count=$5,finished_at=now() where id=$1`,
		result.RunID, normalizedCursor(batch.Cursor), result.Discovered, result.Managed, result.Missing)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (store *SQLSyncStore) applyAutomaticOnboarding(
	ctx context.Context, tx *sql.Tx, instance Instance, teamID int, policy OnboardingPolicy, external ExternalDevice,
) (bool, error) {
	var identityID int64
	var status string
	var deviceID, suggestedTypeID sql.NullInt64
	var confidence sql.NullFloat64
	var category, typeKey sql.NullString
	err := tx.QueryRowContext(ctx, `
		select identity.id,identity.discovery_status,identity.device_id,identity.suggested_device_type_id,
		       identity.match_confidence,device_type.category,device_type.type_key
		  from device_external_identities identity
		  left join device_types device_type on device_type.id=identity.suggested_device_type_id and device_type.status='active'
		 where identity.project_id=$1 and identity.adapter_id=$2 and identity.external_device_id=$3
		 for update of identity`, instance.ProjectID, instance.ID, external.ExternalID).Scan(
		&identityID, &status, &deviceID, &suggestedTypeID, &confidence, &category, &typeKey,
	)
	if err != nil {
		return false, err
	}
	if deviceID.Valid {
		return false, nil
	}
	matches := []DeviceTypeMatch{}
	if suggestedTypeID.Valid && confidence.Valid && typeKey.Valid {
		matches = append(matches, DeviceTypeMatch{DeviceTypeID: suggestedTypeID.Int64, TypeKey: typeKey.String, Confidence: confidence.Float64})
	}
	decision, err := EvaluateOnboarding(OnboardingInput{
		Policy: policy, ExternalID: external.ExternalID, CurrentStatus: status, Matches: matches,
		IdentityConflict: status == "conflicted",
	})
	if err != nil {
		return false, err
	}
	if decision.Status != status && !decision.CreateDevice {
		_, err = tx.ExecContext(ctx, `update device_external_identities set discovery_status=$4
			where project_id=$1 and adapter_id=$2 and id=$3`, instance.ProjectID, instance.ID, identityID, decision.Status)
		return false, err
	}
	if !decision.CreateDevice || !category.Valid {
		return false, nil
	}
	name := strings.TrimSpace(external.ExternalID)
	for _, key := range []string{"callsign", "name", "serialNumber"} {
		if value, ok := external.Attributes[key].(string); ok && strings.TrimSpace(value) != "" {
			name = strings.TrimSpace(value)
			break
		}
	}
	metadata, err := json.Marshal(map[string]any{
		"connectorInstanceId": instance.ID, "externalIdentityId": identityID,
		"onboarding": "automatic", "matchConfidence": decision.Confidence,
	})
	if err != nil {
		return false, err
	}
	var createdDeviceID int64
	err = tx.QueryRowContext(ctx, `
		insert into devices(project_id,adapter_id,device_type_id,name,type,status,metadata_json)
		values($1,$2,$3,$4,$5,'unknown',$6) returning id`,
		instance.ProjectID, instance.ID, decision.DeviceTypeID, name, category.String, metadata).Scan(&createdDeviceID)
	if err != nil {
		return false, err
	}
	updated, err := tx.ExecContext(ctx, `
		update device_external_identities set device_id=$4,discovery_status='managed',bound_at=now()
		 where project_id=$1 and adapter_id=$2 and id=$3 and device_id is null and discovery_status='discovered'`,
		instance.ProjectID, instance.ID, identityID, createdDeviceID)
	if err != nil {
		return false, err
	}
	rows, err := updated.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows != 1 {
		return false, errors.New("connector onboarding identity changed concurrently")
	}
	role := "direct"
	if external.ParentExternalID != "" {
		role = "inherited"
	} else if category.String == "dock" || category.String == "gateway" {
		role = "gateway"
	}
	_, err = tx.ExecContext(ctx, `
		insert into device_connector_bindings(
		  project_id,team_id,device_id,connector_instance_id,external_identity_id,route_role,priority,status,metadata_json
		) values($1,$2,$3,$4,$5,$6,100,'active',$7)`, instance.ProjectID, teamID, createdDeviceID,
		instance.ID, identityID, role, json.RawMessage(`{"source":"automatic-onboarding"}`))
	return err == nil, err
}

func (store *SQLSyncStore) materializeAutomaticRelationships(
	ctx context.Context, tx *sql.Tx, instance Instance, teamID int, devices []ExternalDevice,
) error {
	for _, device := range devices {
		if device.ParentExternalID == "" {
			continue
		}
		_, err := tx.ExecContext(ctx, `
			insert into device_relationships(
			  project_id,team_id,from_device_id,to_device_id,relation_type,source_type,metadata_json
			)
			select $1,$2,parent.device_id,child.device_id,'contains','discovery',$5
			  from device_external_identities parent,device_external_identities child
			 where parent.project_id=$1 and child.project_id=$1
			   and parent.adapter_id=$3 and child.adapter_id=$3
			   and parent.external_device_id=$4 and child.external_device_id=$6
			   and parent.device_id is not null and child.device_id is not null
			   and not exists (
			     select 1 from device_relationships relation
			      where relation.project_id=$1 and relation.from_device_id=parent.device_id
			        and relation.to_device_id=child.device_id and relation.valid_until is null
			   )`, instance.ProjectID, teamID, instance.ID, device.ParentExternalID,
			json.RawMessage(`{"source":"automatic-onboarding"}`), device.ExternalID)
		if err != nil {
			return err
		}
	}
	return nil
}

func normalizedCursor(cursor json.RawMessage) json.RawMessage {
	if len(cursor) == 0 {
		return json.RawMessage(`{}`)
	}
	return cursor
}

func externalIDs(devices []ExternalDevice) []string {
	ids := make([]string, 0, len(devices))
	for _, device := range devices {
		ids = append(ids, device.ExternalID)
	}
	return ids
}
