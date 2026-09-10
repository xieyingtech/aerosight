package adapter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type Discovery struct {
	ProjectID          int
	AdapterID          int64
	ExternalDeviceID   string
	ExternalDeviceType string
	Identity           map[string]any
}

type Registry struct {
	db *sql.DB
}

func NewRegistry(db *sql.DB) *Registry {
	return &Registry{db: db}
}

func (registry *Registry) RecordDiscovery(ctx context.Context, discovery Discovery) (int64, error) {
	if discovery.ProjectID <= 0 || discovery.AdapterID <= 0 || discovery.ExternalDeviceID == "" {
		return 0, errors.New("invalid device discovery")
	}
	identity, err := json.Marshal(discovery.Identity)
	if err != nil {
		return 0, err
	}
	var identityID int64
	err = registry.db.QueryRowContext(ctx, `
		insert into device_external_identities (
		  project_id, team_id, adapter_id, external_device_id, external_device_type, identity_json
		)
		select adapter.project_id, adapter.team_id, adapter.id, $3, $4, $5
		from device_adapters adapter
		where adapter.id = $1 and adapter.project_id = $2
		on conflict (adapter_id, external_device_id) do update
		set external_device_type = excluded.external_device_type,
		    identity_json = excluded.identity_json,
		    last_seen_at = now()
		returning id`, discovery.AdapterID, discovery.ProjectID, discovery.ExternalDeviceID,
		discovery.ExternalDeviceType, identity).Scan(&identityID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("adapter discovery scope mismatch")
	}
	return identityID, err
}
