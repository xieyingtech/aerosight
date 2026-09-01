package dji

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aerosight/worker/internal/adapter"
	"aerosight/worker/internal/driver"
	"aerosight/worker/internal/outbox"
)

type canonicalPayload struct {
	Protocol      string          `json:"protocol"`
	RouteKind     string          `json:"routeKind"`
	TransactionID string          `json:"transactionId"`
	BusinessID    string          `json:"businessId"`
	Method        string          `json:"method"`
	Data          json.RawMessage `json:"data"`
}

type Projector struct{}

func NewProjector() *Projector { return &Projector{} }

func (projector *Projector) Handler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var envelope adapter.UpstreamEnvelope
	if err := json.Unmarshal(event.Payload, &envelope); err != nil {
		return fmt.Errorf("decode DJI upstream envelope: %w", err)
	}
	if err := envelope.ValidateForScope(event.ProjectID, envelope.AdapterID); err != nil {
		return err
	}
	if event.ProjectID != envelope.ProjectID {
		return errors.New("DJI_PROJECTION_PROJECT_SCOPE_MISMATCH")
	}
	var payload canonicalPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("decode DJI canonical payload: %w", err)
	}
	if payload.Protocol != "dji-cloud-api" || len(payload.Data) == 0 {
		return errors.New("DJI_PROJECTION_PAYLOAD_INVALID")
	}
	switch envelope.EventType {
	case "device.topology":
		return projector.projectTopology(ctx, tx, event.TeamID, envelope, payload.Data)
	case "device.state", "device.telemetry":
		return projector.projectRealtime(ctx, tx, event.TeamID, envelope, payload.Data)
	default:
		return fmt.Errorf("DJI_PROJECTION_EVENT_UNSUPPORTED: %s", envelope.EventType)
	}
}

func (projector *Projector) projectTopology(ctx context.Context, tx *sql.Tx, teamID int, envelope adapter.UpstreamEnvelope, data json.RawMessage) error {
	var identity struct {
		Type enumValue `json:"type"`
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		return fmt.Errorf("decode DJI topology family: %w", err)
	}
	var nodes []ProductNode
	var err error
	switch int(identity.Type) {
	case 2:
		nodes, err = ExpandDock2Topology(envelope.ExternalDeviceID, data)
	case 3:
		nodes, err = ExpandDock3Topology(envelope.ExternalDeviceID, data)
	default:
		nodes = []ProductNode{unknownProductNode(envelope.ExternalDeviceID, "", "", "", ProductKey{Domain: 3, Type: int(identity.Type)})}
	}
	if err != nil {
		return err
	}
	claimed := make(map[string]int, len(nodes))
	for _, node := range nodes {
		deviceID, err := projector.claimNode(ctx, tx, teamID, envelope, node)
		if err != nil {
			return err
		}
		claimed[node.ExternalID] = deviceID
	}
	for _, node := range nodes {
		if node.ParentExternalID == "" {
			continue
		}
		parentID, parentExists := claimed[node.ParentExternalID]
		childID, childExists := claimed[node.ExternalID]
		if !parentExists || !childExists {
			return errors.New("DJI_TOPOLOGY_RELATION_IDENTITY_MISSING")
		}
		if _, err := tx.ExecContext(ctx, `
			insert into device_relationships (
			  project_id, team_id, from_device_id, to_device_id, relation_type, source_type,
			  metadata_json
			)
			select $1,$2,$3,$4,$5,'discovery',$6
			where not exists (
			  select 1 from device_relationships
			  where project_id=$1 and from_device_id=$3 and to_device_id=$4
			    and relation_type=$5 and valid_until is null
			)`, envelope.ProjectID, teamID, parentID, childID, node.Relation,
			jsonObject(map[string]any{"adapterId": envelope.AdapterID, "protocol": "dji-cloud-api"})); err != nil {
			return err
		}
	}
	return nil
}

func (projector *Projector) claimNode(ctx context.Context, tx *sql.Tx, teamID int, envelope adapter.UpstreamEnvelope, node ProductNode) (int, error) {
	identityJSON := jsonObject(map[string]any{
		"protocol": "dji-cloud-api", "productDomain": node.ProductKey.Domain,
		"productType": node.ProductKey.Type, "productSubtype": node.ProductKey.Subtype,
		"protocolVersion": node.ProtocolVersion, "firmwareVersion": node.FirmwareVersion, "compatibilityReason": node.CompatibilityReason,
	})
	var deviceID sql.NullInt64
	var identityID int64
	var existingType sql.NullString
	err := tx.QueryRowContext(ctx, `
		insert into device_external_identities (
		  project_id, team_id, adapter_id, external_device_id, external_device_type, identity_json,
		  first_seen_at, last_seen_at
		) values ($1,$2,$3,$4,$5,$6,$7,$7)
		on conflict (adapter_id, external_device_id) do update
		set last_seen_at=greatest(device_external_identities.last_seen_at, excluded.last_seen_at),
		    identity_json=device_external_identities.identity_json || excluded.identity_json
		returning id,device_id,external_device_type`, envelope.ProjectID, teamID, envelope.AdapterID,
		node.ExternalID, node.TypeKey, identityJSON, envelope.CapturedAt).Scan(&identityID, &deviceID, &existingType)
	if err != nil {
		return 0, fmt.Errorf("upsert DJI external identity: %w", err)
	}
	if deviceID.Valid {
		if existingType.Valid && existingType.String != node.TypeKey {
			return 0, fmt.Errorf("DJI_IDENTITY_TYPE_IMMUTABLE: %s is %s, discovered as %s", node.ExternalID, existingType.String, node.TypeKey)
		}
		if err := projector.materializeCapabilities(ctx, tx, teamID, envelope, int(deviceID.Int64), node.ExternalID, node.TypeKey, node.ReadOnly, node.CompatibilityReason); err != nil {
			return 0, err
		}
		if err := projector.ensureConnectorBinding(ctx, tx, teamID, envelope, node, identityID, deviceID.Int64); err != nil {
			return 0, err
		}
		return int(deviceID.Int64), nil
	}
	var deviceTypeID int64
	if err := tx.QueryRowContext(ctx,
		"select id from device_types where type_key=$1 and version=1 and status='active'", node.TypeKey,
	).Scan(&deviceTypeID); err != nil {
		return 0, fmt.Errorf("resolve DJI DeviceType %s: %w", node.TypeKey, err)
	}
	status, freshness, reason := "unknown", "unknown", ""
	if node.ReadOnly {
		status, freshness, reason = "degraded", "fresh", node.CompatibilityReason
	}
	name := fmt.Sprintf("%s · %s", node.Name, node.ExternalID)
	err = tx.QueryRowContext(ctx, `
		insert into devices (
		  project_id, name, type, status, last_seen_at, config_json, metadata_json, adapter_id,
		  device_model, firmware_version, status_reason, device_type_id,
		  status_observed_at, status_projected_at, data_freshness, raw_status_ref
		) values ($1,$2,$3,$4,$5::timestamptz,'{}',$6,$7,$8,nullif($9,''),nullif($10,''),$11,$5::timestamptz,now(),$12,$13)
		returning id`, envelope.ProjectID, name, node.Category, status, envelope.CapturedAt,
		jsonObject(map[string]any{"externalDeviceId": node.ExternalID, "driver": DriverKey}), envelope.AdapterID,
		node.Name, node.FirmwareVersion, reason, deviceTypeID, freshness, envelope.EventID).Scan(&deviceID)
	if err != nil {
		return 0, fmt.Errorf("insert unified DJI device: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		update device_external_identities
		set device_id=$1,bound_at=now(),last_seen_at=greatest(last_seen_at,$2),
		    suggested_device_type_id=$5,match_confidence=1,discovery_status='managed'
		where adapter_id=$3 and external_device_id=$4 and device_id is null`,
		deviceID.Int64, envelope.CapturedAt, envelope.AdapterID, node.ExternalID, deviceTypeID); err != nil {
		return 0, fmt.Errorf("bind DJI external identity: %w", err)
	}
	if err := projector.ensureConnectorBinding(ctx, tx, teamID, envelope, node, identityID, deviceID.Int64); err != nil {
		return 0, err
	}
	if err := projector.materializeCapabilities(ctx, tx, teamID, envelope, int(deviceID.Int64), node.ExternalID, node.TypeKey, node.ReadOnly, node.CompatibilityReason); err != nil {
		return 0, err
	}
	return int(deviceID.Int64), nil
}

func (projector *Projector) ensureConnectorBinding(
	ctx context.Context, tx *sql.Tx, teamID int, envelope adapter.UpstreamEnvelope, node ProductNode,
	identityID, deviceID int64,
) error {
	role := "direct"
	if node.ParentExternalID != "" {
		role = "inherited"
	} else if node.Category == "dock" || node.Category == "gateway" {
		role = "gateway"
	}
	if _, err := tx.ExecContext(ctx, `
		update device_external_identities set discovery_status='managed',bound_at=coalesce(bound_at,now())
		where id=$1 and project_id=$2 and adapter_id=$3 and device_id=$4`,
		identityID, envelope.ProjectID, envelope.AdapterID, deviceID); err != nil {
		return fmt.Errorf("mark DJI identity managed: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		insert into device_connector_bindings(
		  project_id,team_id,device_id,connector_instance_id,external_identity_id,
		  route_role,priority,status,metadata_json
		) values($1,$2,$3,$4,$5,$6,100,'active',$7)
		on conflict(device_id,connector_instance_id) do update set
		  external_identity_id=excluded.external_identity_id,route_role=excluded.route_role,
		  status='active',unbound_at=null,metadata_json=excluded.metadata_json`,
		envelope.ProjectID, teamID, deviceID, envelope.AdapterID, identityID, role,
		jsonObject(map[string]any{"source": "dji-topology", "protocol": "dji-cloud-api"})); err != nil {
		return fmt.Errorf("bind DJI connector route: %w", err)
	}
	return nil
}

func (projector *Projector) materializeCapabilities(ctx context.Context, tx *sql.Tx, teamID int, envelope adapter.UpstreamEnvelope, deviceID int, externalID, typeKey string, readOnly bool, reason string) error {
	var profileJSON json.RawMessage
	if err := tx.QueryRowContext(ctx, `
		select capability_profile_json from device_types where type_key=$1 and version=1`, typeKey,
	).Scan(&profileJSON); err != nil {
		return err
	}
	var profile map[string]json.RawMessage
	if err := json.Unmarshal(profileJSON, &profile); err != nil {
		return err
	}
	manifest := Manifest()
	selected := make(map[string]bool, len(profile))
	for _, capability := range manifest.Capabilities {
		if _, exists := profile[capability.Code]; !exists {
			continue
		}
		availability, availabilityReason := "available", ""
		if readOnly && capability.Kind == driver.CapabilityCommand {
			availability, availabilityReason = "unavailable", reason
		}
		if _, err := tx.ExecContext(ctx, `
			insert into device_capabilities (
			  device_id, capability_code, version, params_schema_json, constraints_json, project_id,
			  version_number, declared_by_adapter_id, availability, availability_reason,
			  input_schema_json, output_schema_json, risk_level, source_json
			) values ($1,$2,$3,$4,'{}',$5,1,$6,$7,nullif($8,''),$4,$9,$10,$11)
			on conflict (device_id, capability_code) do update
			set availability=excluded.availability, availability_reason=excluded.availability_reason,
			    source_json=excluded.source_json, updated_at=now()`,
			deviceID, capability.Code, DriverVersion, jsonOrObject(capability.InputSchema), envelope.ProjectID,
			envelope.AdapterID, availability, availabilityReason, jsonOrObject(capability.OutputSchema),
			capability.Risk, jsonObject(map[string]any{"driver": DriverKey, "typeKey": typeKey})); err != nil {
			return err
		}
		selected[capability.Code] = true
	}
	for _, stream := range manifest.Streams {
		if !selected[stream.CapabilityCode] {
			continue
		}
		stableChannelID := fmt.Sprintf("dji-cloud:%d:%s:%s", envelope.AdapterID, externalID, stream.ChannelKey)
		quality := jsonObject(map[string]any{"source": "dji-cloud-api", "qos": 1, "ordering": "capturedAt+sequence", "freshness": "device-status"})
		if _, err := tx.ExecContext(ctx, `
			insert into device_stream_channels (
			  project_id, team_id, device_id, stable_channel_id, capability_code, channel_key, display_name,
			  data_type, schema_json, unit, protocol, quality_json, source_json
			) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,nullif($10,''),'mqtt5',$11,$12)
			on conflict (device_id, channel_key) do update
			set stable_channel_id=excluded.stable_channel_id,capability_code=excluded.capability_code,
			    schema_json=excluded.schema_json,unit=excluded.unit,protocol=excluded.protocol,
			    quality_json=excluded.quality_json,source_json=excluded.source_json,updated_at=now()`,
			envelope.ProjectID, teamID, deviceID, stableChannelID, stream.CapabilityCode, stream.ChannelKey,
			strings.ReplaceAll(stream.ChannelKey, ".", " "), stream.DataType, jsonOrObject(stream.Schema),
			stream.Unit, quality, jsonObject(map[string]any{"driver": DriverKey, "externalDeviceId": externalID})); err != nil {
			return err
		}
	}
	return nil
}

func (projector *Projector) projectRealtime(ctx context.Context, tx *sql.Tx, teamID int, envelope adapter.UpstreamEnvelope, data json.RawMessage) error {
	deviceID, err := claimedDeviceID(ctx, tx, envelope)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		update devices set status='online', status_reason=null, last_seen_at=$3,
		  status_observed_at=$3, status_projected_at=now(), data_freshness='fresh',
		  raw_status_ref=$4, updated_at=now()
		where id=$1 and project_id=$2
		  and (status_observed_at is null or status_observed_at <= $3)`,
		deviceID, envelope.ProjectID, envelope.CapturedAt, envelope.EventID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		update devices child set status='online',status_reason=null,last_seen_at=$3,
		  status_observed_at=$3,status_projected_at=now(),data_freshness='fresh',
		  raw_status_ref=$4,updated_at=now()
		from device_relationships relation
		where relation.project_id=$2 and relation.from_device_id=$1 and relation.to_device_id=child.id
		  and relation.valid_until is null
		  and (child.status_observed_at is null or child.status_observed_at <= $3)`,
		deviceID, envelope.ProjectID, envelope.CapturedAt, envelope.EventID+":parent"); err != nil {
		return err
	}
	telemetryType := "dji.state"
	if envelope.EventType == "device.telemetry" {
		telemetryType = "dji.osd"
	}
	if err := insertTelemetry(ctx, tx, teamID, envelope, deviceID, envelope.EventID, telemetryType, data); err != nil {
		return err
	}
	if envelope.EventType != "device.telemetry" {
		return nil
	}
	sensorPayload, exists := environmentalSensorPayload(data)
	if !exists {
		return nil
	}
	var signature struct {
		GatewaySN string `json:"gatewaySn"`
	}
	if err := json.Unmarshal(envelope.SignatureContext, &signature); err != nil || signature.GatewaySN == "" {
		return nil
	}
	sensorEnvelope := envelope
	sensorEnvelope.ExternalDeviceID = signature.GatewaySN + ":environment"
	sensorID, err := claimedDeviceID(ctx, tx, sensorEnvelope)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	return insertTelemetry(ctx, tx, teamID, sensorEnvelope, sensorID, envelope.EventID+":environment", "dji.environment", sensorPayload)
}

func claimedDeviceID(ctx context.Context, tx *sql.Tx, envelope adapter.UpstreamEnvelope) (int, error) {
	var deviceID int
	err := tx.QueryRowContext(ctx, `
		select device_id from device_external_identities
		where project_id=$1 and adapter_id=$2 and external_device_id=$3 and device_id is not null`,
		envelope.ProjectID, envelope.AdapterID, envelope.ExternalDeviceID).Scan(&deviceID)
	return deviceID, err
}

func insertTelemetry(ctx context.Context, tx *sql.Tx, teamID int, envelope adapter.UpstreamEnvelope, deviceID int, eventID, telemetryType string, payload json.RawMessage) error {
	if _, err := tx.ExecContext(ctx, `
		update devices set status='online',status_reason=null,last_seen_at=$3,
		  status_observed_at=$3,status_projected_at=now(),data_freshness='fresh',
		  raw_status_ref=$4,updated_at=now()
		where id=$1 and project_id=$2
		  and (status_observed_at is null or status_observed_at <= $3)`,
		deviceID, envelope.ProjectID, envelope.CapturedAt, eventID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		insert into telemetry_event_dedup (adapter_id,event_id,project_id,captured_at,received_at)
		values ($1,$2,$3,$4,$5) on conflict (adapter_id,event_id) do nothing`,
		envelope.AdapterID, eventID, envelope.ProjectID, envelope.CapturedAt, envelope.ReceivedAt)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 0 {
		return err
	}
	quality := jsonObject(map[string]any{"source": "dji-cloud-api", "receivedAt": envelope.ReceivedAt})
	if _, err := tx.ExecContext(ctx, `
		insert into device_telemetry (
		  project_id,team_id,adapter_id,device_id,event_id,telemetry_type,
		  sequence_number,captured_at,received_at,payload_json,quality_json
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		envelope.ProjectID, teamID, envelope.AdapterID, deviceID, eventID, telemetryType,
		envelope.Sequence, envelope.CapturedAt, envelope.ReceivedAt, payload, quality); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		insert into device_latest_telemetry (
		  device_id,project_id,adapter_id,event_id,telemetry_type,sequence_number,
		  captured_at,received_at,payload_json,quality_json
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		on conflict (device_id) do update set
		  adapter_id=excluded.adapter_id,event_id=excluded.event_id,telemetry_type=excluded.telemetry_type,
		  sequence_number=excluded.sequence_number,captured_at=excluded.captured_at,
		  received_at=excluded.received_at,payload_json=excluded.payload_json,
		  quality_json=excluded.quality_json,updated_at=now()
		where excluded.captured_at > device_latest_telemetry.captured_at
		   or (excluded.captured_at = device_latest_telemetry.captured_at
		       and coalesce(excluded.sequence_number,-1) > coalesce(device_latest_telemetry.sequence_number,-1))`,
		deviceID, envelope.ProjectID, envelope.AdapterID, eventID, telemetryType, envelope.Sequence,
		envelope.CapturedAt, envelope.ReceivedAt, payload, quality)
	return err
}

func environmentalSensorPayload(data json.RawMessage) (json.RawMessage, bool) {
	var values map[string]json.RawMessage
	if json.Unmarshal(data, &values) != nil {
		return nil, false
	}
	units := map[string]string{
		"environment_temperature": "°C", "temperature": "°C", "humidity": "%RH",
		"wind_speed": "m/s", "rainfall": "enum",
	}
	samples := make(map[string]any)
	for key, unit := range units {
		value, exists := values[key]
		if !exists {
			continue
		}
		var decoded any
		if json.Unmarshal(value, &decoded) == nil {
			samples[key] = map[string]any{"value": decoded, "unit": unit}
		}
	}
	if len(samples) == 0 {
		return nil, false
	}
	return jsonObject(map[string]any{"samples": samples}), true
}

func jsonObject(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func jsonOrObject(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}
