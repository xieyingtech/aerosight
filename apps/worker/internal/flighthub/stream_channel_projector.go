package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"aerosight/worker/internal/connector"
)

const (
	flightHubVideoReadCapability               = "stream.video.read"
	flightHubVideoControlCapability            = "stream.video.control"
	flightHubLiveActionDisabledReason          = "FLIGHTHUB_LIVE_ACTION_DISABLED"
	flightHubLiveFieldAcceptanceRequiredReason = "FLIGHTHUB_LIVE_FIELD_ACCEPTANCE_REQUIRED"
)

var flightHubCameraIndexPattern = regexp.MustCompile(`^[0-9]{1,4}-[0-9]{1,4}-[0-9]{1,4}$`)

func (projector *SQLDeviceHealthProjector) ApplyDeviceStreamChannels(ctx context.Context, instance connector.Instance, poll DeviceStatePoll) error {
	if projector == nil || projector.db == nil || instance.ID <= 0 || instance.ProjectID <= 0 ||
		poll.Device.DeviceID <= 0 || poll.Device.TeamID <= 0 || strings.TrimSpace(poll.Device.Serial) == "" ||
		poll.Snapshot.SN != poll.Device.Serial || poll.ReceivedAt.IsZero() {
		return errors.New("FlightHub stream channel projection scope is invalid")
	}
	channels := poll.Mapped.StreamChannels
	for _, channel := range channels {
		if !flightHubCameraIndexPattern.MatchString(channel.CameraIndex) || strings.TrimSpace(channel.DisplayName) == "" ||
			(channel.Availability != "available" && channel.Availability != "degraded" && channel.Availability != "unavailable") {
			return errors.New("FlightHub stream channel projection is invalid")
		}
	}

	tx, err := projector.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var adapterID int64
	err = tx.QueryRowContext(ctx, `select id from device_adapters where id=$1 and project_id=$2
		and status in('connecting','connected','degraded')
		and ($3='' or (lease_owner=$3 and connection_epoch=$4 and lease_expires_at>=now())) for share`,
		instance.ID, instance.ProjectID, instance.LeaseOwner, instance.LeaseEpoch).Scan(&adapterID)
	if errors.Is(err, sql.ErrNoRows) {
		return connector.ErrConnectorDisabled
	}
	if err != nil {
		return err
	}
	var deviceTypeID int64
	err = tx.QueryRowContext(ctx, `select device_type_id from devices
		where id=$1 and project_id=$2 and adapter_id=$3 for share`, poll.Device.DeviceID, instance.ProjectID, instance.ID).Scan(&deviceTypeID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("FlightHub stream channel device is outside connector scope")
	}
	if err != nil {
		return err
	}

	if len(channels) > 0 {
		controlAvailability, controlReason, availabilityErr := flightHubLiveControlAvailability(
			ctx, tx, instance, poll.Device.DeviceID,
		)
		if availabilityErr != nil {
			return availabilityErr
		}
		capabilities := []struct {
			code         string
			availability string
			reason       string
		}{
			{code: flightHubVideoReadCapability, availability: "available"},
			{code: flightHubVideoControlCapability, availability: controlAvailability, reason: controlReason},
		}
		for _, capability := range capabilities {
			result, capabilityErr := tx.ExecContext(ctx, `insert into device_capabilities(
			device_id,project_id,capability_code,version,declared_by_adapter_id,params_schema_json,
			input_schema_json,output_schema_json,risk_level,source_json,availability,availability_reason
		) select $1,$2,capability->>'code',driver.version,$3,
			coalesce(capability->'inputSchema','{}'::jsonb),coalesce(capability->'inputSchema','{}'::jsonb),
			coalesce(capability->'outputSchema','{}'::jsonb),coalesce(capability->>'risk','low'),$4::jsonb,
			$7,nullif($8,'')
		from device_types device_type
		join driver_definitions driver on driver.id=device_type.driver_definition_id
		cross join lateral jsonb_array_elements(case when jsonb_typeof(driver.manifest_json->'capabilities')='array'
			then driver.manifest_json->'capabilities' else '[]'::jsonb end) capability
		where device_type.id=$5 and capability->>'code'=$6
		on conflict(device_id,capability_code) do update set
			declared_by_adapter_id=excluded.declared_by_adapter_id,params_schema_json=excluded.params_schema_json,
			input_schema_json=excluded.input_schema_json,output_schema_json=excluded.output_schema_json,
			risk_level=excluded.risk_level,source_json=excluded.source_json,
			availability=excluded.availability,availability_reason=excluded.availability_reason,updated_at=now()`,
				poll.Device.DeviceID, instance.ProjectID, instance.ID,
				streamChannelSource(instance.ID, poll.Mapped.MapperVersion), deviceTypeID, capability.code,
				capability.availability, capability.reason)
			if capabilityErr != nil {
				return capabilityErr
			}
			if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
				if rowsErr != nil {
					return rowsErr
				}
				return errors.New("FlightHub stream channel capability is outside driver contract")
			}
		}
	}

	seen := make([]string, 0, len(channels))
	for _, channel := range channels {
		stableID := fmt.Sprintf("dji-flighthub:%d:%d:%s", instance.ID, poll.Device.DeviceID, channel.CameraIndex)
		result, insertErr := tx.ExecContext(ctx, `insert into device_stream_channels(
			project_id,team_id,device_id,stable_channel_id,capability_code,channel_key,display_name,
			data_type,schema_json,protocol,quality_json,availability,availability_reason,source_json
		) values($1,$2,$3,$4,$5,$6,$7,'video',$8::jsonb,'dji-flighthub-openapi',$9::jsonb,$10,nullif($11,''),$12::jsonb)
		on conflict(device_id,channel_key) do update set
			stable_channel_id=excluded.stable_channel_id,capability_code=excluded.capability_code,
			display_name=excluded.display_name,data_type=excluded.data_type,schema_json=excluded.schema_json,
			protocol=excluded.protocol,quality_json=excluded.quality_json,availability=excluded.availability,
			availability_reason=excluded.availability_reason,source_json=excluded.source_json,updated_at=now()
		where device_stream_channels.project_id=excluded.project_id
			and device_stream_channels.source_json->>'connectorKey'='dji.flighthub2'
			and device_stream_channels.source_json->>'connectorInstanceId'=$13`,
			instance.ProjectID, poll.Device.TeamID, poll.Device.DeviceID, stableID, flightHubVideoReadCapability,
			channel.CameraIndex, channel.DisplayName, `{"type":"object"}`,
			streamChannelQuality(poll.Mapped.MapperVersion), channel.Availability, channel.AvailabilityReason,
			streamChannelSource(instance.ID, poll.Mapped.MapperVersion), fmt.Sprint(instance.ID))
		if insertErr != nil {
			return insertErr
		}
		if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
			if rowsErr != nil {
				return rowsErr
			}
			return errors.New("FlightHub stream channel conflicts with another source")
		}
		seen = append(seen, channel.CameraIndex)
	}
	if _, err := tx.ExecContext(ctx, `update device_stream_channels set
		availability='unavailable',availability_reason='channel_not_reported',updated_at=now()
		where project_id=$1 and device_id=$2 and source_json->>'connectorKey'='dji.flighthub2'
			and source_json->>'connectorInstanceId'=$3
			and not(channel_key=any($4::text[]))`,
		instance.ProjectID, poll.Device.DeviceID, fmt.Sprint(instance.ID), seen); err != nil {
		return err
	}
	return tx.Commit()
}

func flightHubLiveControlAvailability(ctx context.Context, tx *sql.Tx, instance connector.Instance, deviceID int) (string, string, error) {
	var featureEnabled, fieldAccepted bool
	err := tx.QueryRowContext(ctx, `select
		coalesce((flags.flighthub_action_flags_json->>'live.control')::boolean,false),
		exists(select 1 from connector_capability_snapshots capability
			where capability.project_id=adapter.project_id and capability.connector_instance_id=adapter.id
				and capability.capability_code='live.control' and capability.status='supported'
				and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
				and capability.region='cn' and capability.deployment='cn-public-cloud'
				and capability.evidence_level='field-write'
				and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version
				and (capability.expires_at is null or capability.expires_at>now()))
	from device_adapters adapter
	join devices device on device.adapter_id=adapter.id and device.project_id=adapter.project_id
	left join project_feature_flags flags on flags.project_id=adapter.project_id
	where adapter.id=$1 and adapter.project_id=$2 and device.id=$3`,
		instance.ID, instance.ProjectID, deviceID).Scan(&featureEnabled, &fieldAccepted)
	if err != nil {
		return "", "", err
	}
	if !featureEnabled {
		return "unavailable", flightHubLiveActionDisabledReason, nil
	}
	if !fieldAccepted {
		return "unavailable", flightHubLiveFieldAcceptanceRequiredReason, nil
	}
	return "available", "", nil
}

func streamChannelSource(connectorID int64, mapperVersion string) string {
	value, _ := json.Marshal(map[string]any{
		"connectorKey": "dji.flighthub2", "connectorInstanceId": fmt.Sprint(connectorID),
		"mapperVersion": mapperVersion, "source": "dji-flighthub-openapi",
	})
	return string(value)
}

func streamChannelQuality(mapperVersion string) string {
	value, _ := json.Marshal(map[string]any{
		"source": "dji-flighthub-openapi", "mapperVersion": mapperVersion,
		"freshness": "device-state", "cameraIndexContract": "model-profile",
	})
	return string(value)
}
