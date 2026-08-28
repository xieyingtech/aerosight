package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var ErrSyncCursorAdvanced = errors.New("connector sync cursor advanced concurrently")

type SyncApplyResult struct {
	RunID          int64
	Discovered     int
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
		select sync_cursor_json
		  from connector_instances
		 where id=$1 and project_id=$2 and connector_key=$3 and connector_version=$4`,
		instance.ID, instance.ProjectID, instance.ConnectorKey, instance.Version).Scan(&cursor)
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
	var currentCursor, scope []byte
	err = tx.QueryRowContext(ctx, `
		select team_id, sync_cursor_json, discovery_scope_json
		  from device_adapters where id=$1 and project_id=$2 for update`,
		instance.ID, instance.ProjectID).Scan(&teamID, &currentCursor, &scope)
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
				  project_id,team_id,adapter_id,external_device_id,external_device_type,identity_json,
				  discovery_status,source_version,last_sync_run_id,first_seen_at,last_seen_at
				) values ($1,$2,$3,$4,$5,$6,'discovered',$7,$8,now(),now())
				on conflict (adapter_id,external_device_id) do update set
				  external_device_type=excluded.external_device_type,
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
		  discovered_count=$3,missing_count=$4,finished_at=now() where id=$1`,
		result.RunID, normalizedCursor(batch.Cursor), result.Discovered, result.Missing)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
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
