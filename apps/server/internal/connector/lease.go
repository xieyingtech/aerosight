package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type InstanceLease struct {
	Instance
	TeamID int
	Epoch  int64
}

type LeaseRepository interface {
	ClaimDue(context.Context, string, string, string, time.Time, int, time.Duration) ([]InstanceLease, error)
	ClaimInstance(context.Context, string, int, int64, string, string, time.Duration) (InstanceLease, bool, error)
	Renew(context.Context, InstanceLease, string, time.Duration) (bool, error)
	Release(context.Context, InstanceLease, string) error
	Backlog(context.Context, string, string) (int, error)
}

type SQLLeaseRepository struct{ db *sql.DB }

func NewSQLLeaseRepository(db *sql.DB) *SQLLeaseRepository { return &SQLLeaseRepository{db: db} }

const leaseProjection = `adapter.id, adapter.project_id, adapter.team_id, adapter.connection_epoch,
       definition.connector_key, definition.version, adapter.config_json,
       adapter.credential_envelope_json, adapter.discovery_scope_json, adapter.lease_owner`

func scanLease(scanner interface{ Scan(...any) error }) (InstanceLease, error) {
	var lease InstanceLease
	var envelope []byte
	err := scanner.Scan(
		&lease.ID, &lease.ProjectID, &lease.TeamID, &lease.Epoch,
		&lease.ConnectorKey, &lease.Version, &lease.Config,
		&envelope, &lease.DiscoveryScope, &lease.LeaseOwner,
	)
	lease.LeaseEpoch = lease.Epoch
	lease.CredentialEnvelope = json.RawMessage(envelope)
	return lease, err
}

func (repository *SQLLeaseRepository) ClaimDue(
	ctx context.Context,
	owner, connectorKey, version string,
	dueBefore time.Time,
	limit int,
	duration time.Duration,
) ([]InstanceLease, error) {
	if owner == "" || connectorKey == "" || version == "" || limit <= 0 || duration <= 0 {
		return nil, errors.New("connector lease claim is invalid")
	}
	rows, err := repository.db.QueryContext(ctx, `
		with candidates as (
			select adapter.id
			  from device_adapters adapter
			  join connector_definitions definition on definition.id=adapter.connector_definition_id
			 where definition.connector_key=$1 and definition.version=$2 and definition.status='active'
			   and adapter.status in ('connecting','connected','degraded')
			   and (adapter.last_checked_at is null or adapter.last_checked_at <= $3)
			   and (adapter.lease_expires_at is null or adapter.lease_expires_at < now())
			 order by adapter.last_checked_at nulls first, adapter.id
			 for update of adapter skip locked
			 limit $4
		)
		update device_adapters adapter
		   set lease_owner=$5, lease_expires_at=now()+($6*interval '1 millisecond'),
		       connection_epoch=adapter.connection_epoch+1, updated_at=now()
		  from candidates, connector_definitions definition
		 where adapter.id=candidates.id and definition.id=adapter.connector_definition_id
		returning `+leaseProjection,
		connectorKey, version, dueBefore, limit, owner, duration.Milliseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	leases := make([]InstanceLease, 0)
	for rows.Next() {
		lease, scanErr := scanLease(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		leases = append(leases, lease)
	}
	return leases, rows.Err()
}

func (repository *SQLLeaseRepository) ClaimInstance(
	ctx context.Context,
	owner string,
	projectID int,
	instanceID int64,
	connectorKey, version string,
	duration time.Duration,
) (InstanceLease, bool, error) {
	if owner == "" || projectID <= 0 || instanceID <= 0 || connectorKey == "" || version == "" || duration <= 0 {
		return InstanceLease{}, false, errors.New("connector instance lease claim is invalid")
	}
	row := repository.db.QueryRowContext(ctx, `
		update device_adapters adapter
		   set lease_owner=$1, lease_expires_at=now()+($6*interval '1 millisecond'),
		       connection_epoch=adapter.connection_epoch+1, updated_at=now()
		  from connector_definitions definition
		 where adapter.id=$2 and adapter.project_id=$3
		   and definition.id=adapter.connector_definition_id
		   and definition.connector_key=$4 and definition.version=$5 and definition.status='active'
		   -- Explicit outbox work may resume after a terminal failure so a worker crash
		   -- between recording the failed outcome and completing the event cannot turn
		   -- into repeated CONNECTOR_LEASE_UNAVAILABLE retries. Periodic ClaimDue still
		   -- excludes failed connectors.
		   and adapter.status in ('connecting','connected','degraded','failed')
		   and (adapter.lease_expires_at is null or adapter.lease_expires_at < now())
		returning `+leaseProjection,
		owner, instanceID, projectID, connectorKey, version, duration.Milliseconds())
	lease, err := scanLease(row)
	if errors.Is(err, sql.ErrNoRows) {
		return InstanceLease{}, false, nil
	}
	return lease, err == nil, err
}

func (repository *SQLLeaseRepository) Renew(
	ctx context.Context,
	lease InstanceLease,
	owner string,
	duration time.Duration,
) (bool, error) {
	result, err := repository.db.ExecContext(ctx, `
		update device_adapters
		   set lease_expires_at=now()+($5*interval '1 millisecond'), updated_at=now()
		 where id=$1 and project_id=$2 and connection_epoch=$3 and lease_owner=$4
		   and status in ('connecting','connected','degraded') and lease_expires_at>=now()`,
		lease.ID, lease.ProjectID, lease.Epoch, owner, duration.Milliseconds())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (repository *SQLLeaseRepository) Release(ctx context.Context, lease InstanceLease, owner string) error {
	_, err := repository.db.ExecContext(ctx, `
		update device_adapters set lease_owner=null,lease_expires_at=null,updated_at=now()
		 where id=$1 and project_id=$2 and connection_epoch=$3 and lease_owner=$4`,
		lease.ID, lease.ProjectID, lease.Epoch, owner)
	return err
}

func (repository *SQLLeaseRepository) Backlog(ctx context.Context, connectorKey, version string) (int, error) {
	var count int
	err := repository.db.QueryRowContext(ctx, `
		select count(*)
		  from outbox_events event
		 where event.event_type='connector.sync.requested'
		   and event.status in ('pending','processing')
		   and event.payload_json->>'connectorKey'=$1
		   and event.payload_json->>'connectorInstanceId' ~ '^[0-9]+$'
		   and exists (
		     select 1 from device_adapters adapter
		     join connector_definitions definition on definition.id=adapter.connector_definition_id
		      where adapter.id=case
		              when event.payload_json->>'connectorInstanceId' ~ '^[0-9]+$'
		              then (event.payload_json->>'connectorInstanceId')::bigint
		            end
		        and adapter.project_id=event.project_id and adapter.status<>'disabled'
		        and definition.connector_key=$1 and definition.version=$2
		   )`, connectorKey, version).Scan(&count)
	return count, err
}
