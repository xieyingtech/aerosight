package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRemoteResourceUnavailable = errors.New("CONNECTOR_REMOTE_RESOURCE_UNAVAILABLE")
	ErrConnectorDisabled         = errors.New("CONNECTOR_DISABLED")
	resourceKinds                = stringSet(
		"wayline", "flight-task", "flight-media", "flight-record", "flight-alert", "ai-alert",
		"map-element", "flight-area", "offline-map", "air-sense-warning", "model", "model-resource",
		"live-share", "stream-converter", "recording", "hms", "topology", "auto-record",
		"organization-user", "organization-role", "organization-permission",
	)
	syncResourceKinds = stringSet(
		"inventory", "device-state", "health", "active-operations", "waylines", "flight-tasks",
		"flight-artifacts", "live", "geospatial", "models", "organization",
	)
	capabilityStatuses = stringSet("supported", "empty", "forbidden", "not_applicable", "unverified", "degraded", "failed")
	evidenceLevels     = stringSet("documented", "fixture", "live-read", "field-write")
)

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validSetValue(values map[string]struct{}, value string) bool {
	_, ok := values[value]
	return ok
}

type CanonicalResourceLink struct {
	TargetType string
	TargetID   string
}

type RemoteResource struct {
	RemoteID        string
	RemoteVersion   string
	RemoteUpdatedAt *time.Time
	Summary         map[string]any
	Canonical       *CanonicalResourceLink
}

type RemoteResourceBatch struct {
	Kind             string
	Resources        []RemoteResource
	CompleteSnapshot bool
}

type RemoteResourceApplyResult struct {
	Upserted int
	Missing  int64
}

type ResourceSyncUpdate struct {
	Kind          string
	Status        string
	Cursor        map[string]any
	AttemptCount  int
	LastErrorCode string
	StartedAt     *time.Time
	SucceededAt   *time.Time
	NextAttemptAt *time.Time
}

type ManagedConnectorDevice struct {
	DeviceID   int
	TeamID     int
	ExternalID string
	Serial     string
	ModelKey   string
	Class      string
	Online     bool
}

type CapabilitySnapshot struct {
	CapabilityCode  string
	Status          string
	EvidenceLevel   string
	Region          string
	Deployment      string
	DeviceModel     string
	FirmwareVersion string
	Details         map[string]any
	VerifiedAt      time.Time
	ExpiresAt       *time.Time
}

type SQLResourceRepository struct{ db *sql.DB }

func NewSQLResourceRepository(db *sql.DB) *SQLResourceRepository {
	return &SQLResourceRepository{db: db}
}

func validateInstance(instance Instance) error {
	if instance.ID <= 0 || instance.ProjectID <= 0 {
		return errors.New("connector instance identity is invalid")
	}
	return nil
}

func (repository *SQLResourceRepository) AssertWritable(ctx context.Context, instance Instance) error {
	if repository == nil || repository.db == nil {
		return errors.New("connector resource repository is unavailable")
	}
	if err := validateInstance(instance); err != nil {
		return err
	}
	var status string
	var leaseValid bool
	err := repository.db.QueryRowContext(ctx, `select adapter.status,
		($3='' or (adapter.lease_owner=$3 and adapter.connection_epoch=$4 and adapter.lease_expires_at>=now()))
		from device_adapters adapter where adapter.id=$1 and adapter.project_id=$2`,
		instance.ID, instance.ProjectID, instance.LeaseOwner, instance.LeaseEpoch).Scan(&status, &leaseValid)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRemoteResourceUnavailable
	}
	if err != nil {
		return err
	}
	if !validSetValue(stringSet("connecting", "connected", "degraded"), status) || !leaseValid {
		return ErrConnectorDisabled
	}
	return nil
}

func validateRemoteBatch(batch RemoteResourceBatch) error {
	if !validSetValue(resourceKinds, batch.Kind) {
		return fmt.Errorf("unsupported connector remote resource kind %q", batch.Kind)
	}
	seen := make(map[string]struct{}, len(batch.Resources))
	for _, resource := range batch.Resources {
		remoteID := strings.TrimSpace(resource.RemoteID)
		if remoteID == "" || len(remoteID) > 512 || remoteID != resource.RemoteID {
			return errors.New("connector remote resource identity is invalid")
		}
		if _, duplicate := seen[remoteID]; duplicate {
			return fmt.Errorf("duplicate connector remote resource %q", remoteID)
		}
		seen[remoteID] = struct{}{}
		if resource.Canonical != nil && (strings.TrimSpace(resource.Canonical.TargetType) == "" || strings.TrimSpace(resource.Canonical.TargetID) == "") {
			return errors.New("canonical resource link is incomplete")
		}
	}
	return nil
}

func (repository *SQLResourceRepository) ApplyRemoteResources(
	ctx context.Context, instance Instance, batch RemoteResourceBatch,
) (result RemoteResourceApplyResult, returnedErr error) {
	if repository == nil || repository.db == nil {
		return result, errors.New("connector resource repository is unavailable")
	}
	if err := validateInstance(instance); err != nil {
		return result, err
	}
	if err := repository.AssertWritable(ctx, instance); err != nil {
		return result, err
	}
	if err := validateRemoteBatch(batch); err != nil {
		return result, err
	}
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	var teamID int
	err = tx.QueryRowContext(ctx, `select team_id from device_adapters where id=$1 and project_id=$2
		and status in('connecting','connected','degraded')
		and ($3='' or (lease_owner=$3 and connection_epoch=$4 and lease_expires_at>=now())) for update`,
		instance.ID, instance.ProjectID, instance.LeaseOwner, instance.LeaseEpoch).Scan(&teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrRemoteResourceUnavailable
	}
	if err != nil {
		return result, err
	}
	seen := make([]string, 0, len(batch.Resources))
	for _, resource := range batch.Resources {
		summary, marshalErr := json.Marshal(firstNonNilMap(resource.Summary))
		if marshalErr != nil {
			return result, marshalErr
		}
		var targetType, targetID any
		if resource.Canonical != nil {
			targetType, targetID = resource.Canonical.TargetType, resource.Canonical.TargetID
		}
		_, err = tx.ExecContext(ctx, `
			insert into connector_remote_resources(
			  project_id,team_id,connector_instance_id,resource_kind,remote_id,remote_version,
			  remote_updated_at,status,summary_json,canonical_target_type,canonical_target_id,
			  first_seen_at,last_seen_at,missing_at,created_at,updated_at
			) values($1,$2,$3,$4,$5,$6,$7,'active',$8,$9,$10,now(),now(),null,now(),now())
			on conflict(project_id,connector_instance_id,resource_kind,remote_id) do update set
			  remote_version=excluded.remote_version,remote_updated_at=excluded.remote_updated_at,
			  status='active',summary_json=excluded.summary_json,
			  canonical_target_type=coalesce(excluded.canonical_target_type,connector_remote_resources.canonical_target_type),
			  canonical_target_id=coalesce(excluded.canonical_target_id,connector_remote_resources.canonical_target_id),
			  last_seen_at=now(),missing_at=null,updated_at=now()`,
			instance.ProjectID, teamID, instance.ID, batch.Kind, resource.RemoteID, nullableText(resource.RemoteVersion),
			resource.RemoteUpdatedAt, summary, targetType, targetID)
		if err != nil {
			return result, err
		}
		seen = append(seen, resource.RemoteID)
		result.Upserted++
	}
	if batch.CompleteSnapshot {
		seenJSON, marshalErr := json.Marshal(seen)
		if marshalErr != nil {
			return result, marshalErr
		}
		updated, execErr := tx.ExecContext(ctx, `
			update connector_remote_resources resource
			   set status='missing',missing_at=coalesce(missing_at,now()),updated_at=now()
			 where project_id=$1 and connector_instance_id=$2 and resource_kind=$3 and status='active'
			   and not exists(select 1 from jsonb_array_elements_text($4::jsonb) seen(id) where seen.id=resource.remote_id)`,
			instance.ProjectID, instance.ID, batch.Kind, seenJSON)
		if execErr != nil {
			return result, execErr
		}
		result.Missing, err = updated.RowsAffected()
		if err != nil {
			return result, err
		}
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (repository *SQLResourceRepository) LinkRemoteResource(
	ctx context.Context, instance Instance, kind, remoteID string, link CanonicalResourceLink,
) error {
	if err := validateInstance(instance); err != nil {
		return err
	}
	if err := repository.AssertWritable(ctx, instance); err != nil {
		return err
	}
	if !validSetValue(resourceKinds, kind) || strings.TrimSpace(remoteID) == "" || strings.TrimSpace(link.TargetType) == "" || strings.TrimSpace(link.TargetID) == "" {
		return errors.New("connector remote resource link is invalid")
	}
	result, err := repository.db.ExecContext(ctx, `
		update connector_remote_resources
		   set canonical_target_type=$5,canonical_target_id=$6,updated_at=now()
		 where project_id=$1 and connector_instance_id=$2 and resource_kind=$3 and remote_id=$4`,
		instance.ProjectID, instance.ID, kind, remoteID, link.TargetType, link.TargetID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrRemoteResourceUnavailable
	}
	return nil
}

func (repository *SQLResourceRepository) SaveResourceSyncState(
	ctx context.Context, instance Instance, update ResourceSyncUpdate,
) error {
	if err := validateInstance(instance); err != nil {
		return err
	}
	if update.Status != "disabled" {
		if err := repository.AssertWritable(ctx, instance); err != nil {
			return err
		}
	}
	if !validSetValue(syncResourceKinds, update.Kind) || !validSetValue(stringSet("idle", "running", "backoff", "failed", "disabled"), update.Status) || update.AttemptCount < 0 {
		return errors.New("connector resource sync state is invalid")
	}
	cursor, err := json.Marshal(firstNonNilMap(update.Cursor))
	if err != nil {
		return err
	}
	result, err := repository.db.ExecContext(ctx, `
		insert into connector_resource_sync_states(
		  project_id,team_id,connector_instance_id,resource_kind,status,cursor_json,attempt_count,
		  last_error_code,last_started_at,last_succeeded_at,next_attempt_at,created_at,updated_at
		) select $1,adapter.team_id,$2,$3,$4,$5,$6,$7,$8,$9,$10,now(),now()
		    from device_adapters adapter where adapter.id=$2 and adapter.project_id=$1
		      and (adapter.status in('connecting','connected','degraded') or $4='disabled')
		      and ($11='' or (adapter.lease_owner=$11 and adapter.connection_epoch=$12 and adapter.lease_expires_at>=now()))
		on conflict(project_id,connector_instance_id,resource_kind) do update set
		  status=excluded.status,cursor_json=excluded.cursor_json,attempt_count=excluded.attempt_count,
		  last_error_code=excluded.last_error_code,last_started_at=excluded.last_started_at,
		  last_succeeded_at=excluded.last_succeeded_at,next_attempt_at=excluded.next_attempt_at,updated_at=now()`,
		instance.ProjectID, instance.ID, update.Kind, update.Status, cursor, update.AttemptCount,
		nullableText(update.LastErrorCode), update.StartedAt, update.SucceededAt, update.NextAttemptAt,
		instance.LeaseOwner, instance.LeaseEpoch)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrRemoteResourceUnavailable
	}
	return nil
}

func (repository *SQLResourceRepository) LoadResourceSyncState(
	ctx context.Context, instance Instance, kind string,
) (ResourceSyncUpdate, bool, error) {
	if err := validateInstance(instance); err != nil {
		return ResourceSyncUpdate{}, false, err
	}
	if !validSetValue(syncResourceKinds, kind) {
		return ResourceSyncUpdate{}, false, errors.New("connector resource sync kind is invalid")
	}
	var update ResourceSyncUpdate
	var cursor []byte
	var lastError sql.NullString
	var startedAt, succeededAt, nextAttemptAt sql.NullTime
	err := repository.db.QueryRowContext(ctx, `
		select resource_kind,status,cursor_json,attempt_count,last_error_code,
		       last_started_at,last_succeeded_at,next_attempt_at
		  from connector_resource_sync_states
		 where project_id=$1 and connector_instance_id=$2 and resource_kind=$3`,
		instance.ProjectID, instance.ID, kind).Scan(
		&update.Kind, &update.Status, &cursor, &update.AttemptCount, &lastError,
		&startedAt, &succeededAt, &nextAttemptAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceSyncUpdate{Kind: kind, Status: "idle", Cursor: map[string]any{}}, false, nil
	}
	if err != nil {
		return ResourceSyncUpdate{}, false, err
	}
	if json.Unmarshal(cursor, &update.Cursor) != nil || update.Cursor == nil {
		return ResourceSyncUpdate{}, false, errors.New("connector resource sync cursor is invalid")
	}
	update.LastErrorCode = lastError.String
	if startedAt.Valid {
		update.StartedAt = &startedAt.Time
	}
	if succeededAt.Valid {
		update.SucceededAt = &succeededAt.Time
	}
	if nextAttemptAt.Valid {
		update.NextAttemptAt = &nextAttemptAt.Time
	}
	return update, true, nil
}

func (repository *SQLResourceRepository) ListManagedDevices(
	ctx context.Context, instance Instance,
) ([]ManagedConnectorDevice, error) {
	if err := validateInstance(instance); err != nil {
		return nil, err
	}
	if err := repository.AssertWritable(ctx, instance); err != nil {
		return nil, err
	}
	rows, err := repository.db.QueryContext(ctx, `
		select identity.device_id,identity.team_id,identity.external_device_id,
		       identity.identity_json#>>'{attributes,serialNumber}',
		       identity.identity_json#>>'{attributes,model,key}',
		       identity.identity_json#>>'{attributes,model,class}',
		       coalesce((identity.identity_json#>>'{attributes,online}')::boolean,false)
		  from device_external_identities identity
		  join device_connector_bindings binding
		    on binding.project_id=identity.project_id
		   and binding.connector_instance_id=identity.adapter_id
		   and binding.external_identity_id=identity.id
		   and binding.device_id=identity.device_id
		 where identity.project_id=$1 and identity.adapter_id=$2
		   and identity.discovery_status='managed' and identity.device_id is not null
		   and binding.status='active'
		 order by identity.device_id`, instance.ProjectID, instance.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := make([]ManagedConnectorDevice, 0)
	for rows.Next() {
		var device ManagedConnectorDevice
		if err := rows.Scan(&device.DeviceID, &device.TeamID, &device.ExternalID, &device.Serial, &device.ModelKey, &device.Class, &device.Online); err != nil {
			return nil, err
		}
		if device.DeviceID <= 0 || device.TeamID <= 0 || strings.TrimSpace(device.ExternalID) == "" || strings.TrimSpace(device.Serial) == "" {
			return nil, errors.New("managed connector device identity is invalid")
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (repository *SQLResourceRepository) SaveCapabilitySnapshot(
	ctx context.Context, instance Instance, snapshot CapabilitySnapshot,
) error {
	if err := validateInstance(instance); err != nil {
		return err
	}
	if err := repository.AssertWritable(ctx, instance); err != nil {
		return err
	}
	if strings.TrimSpace(snapshot.CapabilityCode) == "" || !validSetValue(capabilityStatuses, snapshot.Status) ||
		!validSetValue(evidenceLevels, snapshot.EvidenceLevel) || strings.TrimSpace(snapshot.Region) == "" ||
		strings.TrimSpace(snapshot.Deployment) == "" || snapshot.VerifiedAt.IsZero() ||
		(snapshot.ExpiresAt != nil && !snapshot.ExpiresAt.After(snapshot.VerifiedAt)) {
		return errors.New("connector capability snapshot is invalid")
	}
	details, err := json.Marshal(firstNonNilMap(snapshot.Details))
	if err != nil {
		return err
	}
	result, err := repository.db.ExecContext(ctx, `
		insert into connector_capability_snapshots(
		  project_id,team_id,connector_instance_id,capability_code,status,evidence_level,region,deployment,
		  device_model,firmware_version,details_json,verified_at,expires_at,created_at,updated_at
		) select $1,adapter.team_id,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now(),now()
		    from device_adapters adapter where adapter.id=$2 and adapter.project_id=$1
		on conflict(project_id,connector_instance_id,capability_code,region,deployment,device_model,firmware_version) do update set
		  status=excluded.status,evidence_level=excluded.evidence_level,details_json=excluded.details_json,
		  verified_at=excluded.verified_at,expires_at=excluded.expires_at,updated_at=now()`,
		instance.ProjectID, instance.ID, snapshot.CapabilityCode, snapshot.Status, snapshot.EvidenceLevel,
		snapshot.Region, snapshot.Deployment, nullableText(snapshot.DeviceModel), nullableText(snapshot.FirmwareVersion),
		details, snapshot.VerifiedAt, snapshot.ExpiresAt)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrRemoteResourceUnavailable
	}
	return nil
}

func (repository *SQLResourceRepository) ListCapabilitySnapshots(
	ctx context.Context, instance Instance, region, deployment string,
) ([]CapabilitySnapshot, error) {
	if repository == nil || repository.db == nil {
		return nil, errors.New("connector resource repository is unavailable")
	}
	if err := validateInstance(instance); err != nil {
		return nil, err
	}
	region, deployment = strings.TrimSpace(region), strings.TrimSpace(deployment)
	if region == "" || deployment == "" {
		return nil, errors.New("connector capability snapshot scope is invalid")
	}
	rows, err := repository.db.QueryContext(ctx, `
		select capability_code,status,evidence_level,region,deployment,
		       device_model,firmware_version,details_json,verified_at,expires_at
		  from connector_capability_snapshots
		 where project_id=$1 and connector_instance_id=$2 and region=$3 and deployment=$4
		 order by capability_code,verified_at desc,device_model nulls first,firmware_version nulls first`,
		instance.ProjectID, instance.ID, region, deployment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	snapshots := make([]CapabilitySnapshot, 0)
	for rows.Next() {
		var snapshot CapabilitySnapshot
		var deviceModel, firmwareVersion sql.NullString
		var details []byte
		var expiresAt sql.NullTime
		if err := rows.Scan(
			&snapshot.CapabilityCode, &snapshot.Status, &snapshot.EvidenceLevel, &snapshot.Region, &snapshot.Deployment,
			&deviceModel, &firmwareVersion, &details, &snapshot.VerifiedAt, &expiresAt,
		); err != nil {
			return nil, err
		}
		if json.Unmarshal(details, &snapshot.Details) != nil || snapshot.Details == nil {
			return nil, errors.New("connector capability snapshot details are invalid")
		}
		snapshot.DeviceModel = deviceModel.String
		snapshot.FirmwareVersion = firmwareVersion.String
		if expiresAt.Valid {
			snapshot.ExpiresAt = &expiresAt.Time
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func firstNonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
