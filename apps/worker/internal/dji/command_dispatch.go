package dji

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"aerosight/worker/internal/adapter"
	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

type CommandPublisher interface {
	Publish(context.Context, int64, string, []byte) error
}

type CommandDispatcher struct {
	publisher  CommandPublisher
	now        func() time.Time
	authSecret string
}

func NewCommandDispatcher(publisher CommandPublisher, now func() time.Time, authSecret ...string) (*CommandDispatcher, error) {
	if publisher == nil {
		return nil, errors.New("DJI_COMMAND_PUBLISHER_REQUIRED")
	}
	if now == nil {
		now = time.Now
	}
	secret := ""
	if len(authSecret) > 0 {
		secret = authSecret[0]
	}
	return &CommandDispatcher{publisher: publisher, now: now, authSecret: secret}, nil
}

func (dispatcher *CommandDispatcher) DispatchHandler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var request struct {
		CommandID string `json:"commandId"`
	}
	if json.Unmarshal(event.Payload, &request) != nil || request.CommandID == "" {
		return errors.New("DJI_COMMAND_EVENT_INVALID")
	}
	var targetDeviceID int
	err := tx.QueryRowContext(ctx, `select device_id from device_commands
		where project_id=$1 and id=$2::uuid and status='dispatchable'`, event.ProjectID, request.CommandID).Scan(&targetDeviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	route, err := connector.ResolvePrimaryBinding(ctx, tx, event.ProjectID, targetDeviceID)
	if err != nil {
		if errors.Is(err, connector.ErrRouteUnavailable) {
			return errors.New("DJI_COMMAND_ROUTE_UNAVAILABLE")
		}
		if errors.Is(err, connector.ErrRouteConflict) {
			return errors.New("DJI_COMMAND_ROUTE_CONFLICT")
		}
		return err
	}
	var command struct {
		ID                 string
		CapabilityCode     string
		CommandKey         string
		Parameters         json.RawMessage
		Deadline           time.Time
		Priority           int
		AdapterID          int64
		GatewaySN          string
		ProductFamily      string
		LiveStreamID       sql.NullInt64
		CredentialEnvelope []byte
		MediaIngestBaseURL sql.NullString
		IngestRef          sql.NullString
	}
	err = tx.QueryRowContext(ctx, `
		select command.id::text,command.capability_code,command.command_key,command.parameters_json,
		       command.deadline_at,command.priority,adapter.id,
		       coalesce(device_type.capability_profile_json->'dock.debug.control'->>'productFamily',
		         case gateway.type_key when 'dji.dock2' then 'dock2' when 'dji.dock3' then 'dock3' else '' end),
		       coalesce(gateway.external_device_id,target_identity.external_device_id),
		       command.live_stream_id,adapter.credential_envelope_json,profile.media_ingest_base_url,stream.ingest_ref
		from device_commands command
		join devices device on device.id=command.device_id and device.project_id=command.project_id
		join device_adapters adapter on adapter.id=$3 and adapter.project_id=device.project_id
		left join device_network_profiles profile on profile.id=adapter.network_profile_id and profile.project_id=adapter.project_id
		left join live_streams stream on stream.id=command.live_stream_id and stream.project_id=command.project_id
		join device_types device_type on device_type.id=device.device_type_id
		join driver_definitions driver on driver.id=device_type.driver_definition_id and driver.driver_key='dji.cloud'
		join device_external_identities target_identity
		  on target_identity.device_id=device.id and target_identity.adapter_id=adapter.id
		left join lateral (
		  with recursive ancestors(device_id,depth) as (
		    select device.id,0
		    union all
		    select relation.from_device_id,ancestors.depth+1
		    from ancestors join device_relationships relation
		      on relation.project_id=command.project_id and relation.to_device_id=ancestors.device_id
		     and relation.valid_until is null
		    where ancestors.depth<3
		  )
		  select identity.external_device_id,candidate_type.type_key
		  from ancestors
		  join devices candidate on candidate.id=ancestors.device_id and candidate.project_id=command.project_id
		  join device_types candidate_type on candidate_type.id=candidate.device_type_id
		  join device_external_identities identity
		    on identity.device_id=candidate.id and identity.adapter_id=adapter.id
		  where candidate_type.category='dock'
		  order by ancestors.depth limit 1
		) gateway on true
		where command.project_id=$1 and command.id=$2::uuid and command.status='dispatchable'
		for update of command`, event.ProjectID, request.CommandID, route.ConnectorInstanceID).Scan(
		&command.ID, &command.CapabilityCode, &command.CommandKey, &command.Parameters,
		&command.Deadline, &command.Priority, &command.AdapterID, &command.ProductFamily, &command.GatewaySN,
		&command.LiveStreamID, &command.CredentialEnvelope, &command.MediaIngestBaseURL, &command.IngestRef,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if command.CapabilityCode == "flight.return_home" && command.Priority < 90 {
		return errors.New("DJI_RETURN_HOME_PRIORITY_TOO_LOW")
	}
	now := dispatcher.now().UTC()
	if !now.Before(command.Deadline) {
		_, err := tx.ExecContext(ctx, `update device_commands set status='timed_out',completed_at=$3
			where project_id=$1 and id=$2::uuid and status='dispatchable'`, event.ProjectID, command.ID, now)
		return err
	}
	if command.CommandKey == "start" && command.LiveStreamID.Valid && dispatcher.authSecret != "" {
		command.Parameters, err = injectLivePublishCredentials(command.Parameters, command.CredentialEnvelope,
			dispatcher.authSecret, command.AdapterID, event.ProjectID, command.MediaIngestBaseURL.String, command.IngestRef.String)
		if err != nil {
			return err
		}
	}
	service, err := BuildServiceCommand(command.GatewaySN, command.ID, command.ID, command.ProductFamily,
		command.CapabilityCode, command.CommandKey, command.Parameters, now)
	if err != nil {
		return err
	}
	var correlationStatus string
	err = tx.QueryRowContext(ctx, `
		insert into device_command_protocol_correlations (
		  project_id,team_id,command_id,adapter_id,mapping_version,transaction_id,business_id,
		  method,request_topic,request_payload_json,status
		) values ($1,$2,$3::uuid,$4,$5,$6,$7,$8,$9,$10,'prepared')
		on conflict (command_id) do update set command_id=excluded.command_id
		returning status`, event.ProjectID, event.TeamID, command.ID, command.AdapterID,
		service.MappingVersion, service.TransactionID, service.BusinessID, service.Method,
		service.Topic, redactLiveServicePayload(service.Payload)).Scan(&correlationStatus)
	if err != nil {
		return err
	}
	if correlationStatus != "prepared" {
		return nil
	}
	if err := dispatcher.publisher.Publish(ctx, command.AdapterID, service.Topic, service.Payload); err != nil {
		return fmt.Errorf("publish DJI service command: %w", err)
	}
	var attempt int
	if err := tx.QueryRowContext(ctx, `
		insert into command_attempts(project_id,team_id,command_id,adapter_id,attempt,status,sent_at)
		select $1,$2,$3::uuid,$4,coalesce(max(attempt),0)+1,'sent',$5
		from command_attempts where command_id=$3::uuid returning attempt`,
		event.ProjectID, event.TeamID, command.ID, command.AdapterID, now).Scan(&attempt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		update device_command_protocol_correlations set status='sent',sent_at=$2,updated_at=now()
		where command_id=$1::uuid and status='prepared'`, command.ID, now); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `update device_commands set status='sent',result_json=result_json||$3,completed_at=null
		where project_id=$1 and id=$2::uuid and status='dispatchable'`, event.ProjectID, command.ID,
		jsonObject(map[string]any{"djiMappingVersion": service.MappingVersion, "djiMethod": service.Method, "attempt": attempt}))
	return err
}

type livePublishCredential struct {
	Username string `json:"mediaPublishUser"`
	Password string `json:"mediaPublishPassword"`
}

func injectLivePublishCredentials(parameters, rawEnvelope []byte, authSecret string, adapterID int64, projectID int, baseURL, ingestRef string) (json.RawMessage, error) {
	envelope, err := credentials.ParseEnvelope(rawEnvelope)
	if err != nil {
		return nil, errors.New("DJI_LIVE_CREDENTIAL_UNAVAILABLE")
	}
	var credential livePublishCredential
	if err := credentials.DecryptJSON(envelope, authSecret, credentials.AAD("device-adapter", adapterID, projectID), &credential); err != nil || credential.Username == "" || credential.Password == "" {
		return nil, errors.New("DJI_LIVE_CREDENTIAL_UNAVAILABLE")
	}
	target, err := url.Parse(baseURL)
	if err != nil || (target.Scheme != "rtmp" && target.Scheme != "rtmps") || target.Hostname() == "" || target.User != nil || ingestRef == "" {
		return nil, errors.New("DJI_LIVE_INGEST_PROFILE_INVALID")
	}
	target.Path = strings.TrimRight(target.Path, "/") + "/" + strings.TrimLeft(ingestRef, "/")
	query := target.Query()
	query.Set("user", credential.Username)
	query.Set("pass", credential.Password)
	target.RawQuery = query.Encode()
	var value map[string]any
	if json.Unmarshal(parameters, &value) != nil {
		return nil, errors.New("DJI_LIVE_START_PARAMETERS_INVALID")
	}
	value["url"] = target.String()
	return json.Marshal(value)
}

func redactLiveServicePayload(payload json.RawMessage) json.RawMessage {
	var value map[string]any
	if json.Unmarshal(payload, &value) != nil {
		return json.RawMessage(`{"redacted":true}`)
	}
	if data, ok := value["data"].(map[string]any); ok {
		if _, exists := data["url"]; exists {
			data["url"] = "[REDACTED]"
		}
	}
	redacted, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{"redacted":true}`)
	}
	return redacted
}

func (dispatcher *CommandDispatcher) ReplyHandler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var envelope adapter.UpstreamEnvelope
	if err := json.Unmarshal(event.Payload, &envelope); err != nil {
		return fmt.Errorf("decode DJI reply envelope: %w", err)
	}
	if envelope.EventType != "command.reply" || envelope.ProjectID != event.ProjectID {
		return errors.New("DJI_REPLY_SCOPE_INVALID")
	}
	if err := envelope.ValidateForScope(event.ProjectID, envelope.AdapterID); err != nil {
		return err
	}
	var payload canonicalPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("decode DJI reply payload: %w", err)
	}
	if payload.Protocol != "dji-cloud-api" || payload.RouteKind != string(RouteServiceReply) {
		return errors.New("DJI_REPLY_PROTOCOL_INVALID")
	}
	reply, err := DecodeServiceReply(payload.Data, payload.TransactionID, payload.BusinessID, payload.Method)
	if err != nil {
		return err
	}
	var commandID, status, commandKey string
	var liveStreamID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		select correlation.command_id::text,correlation.status,command.command_key,command.live_stream_id
		from device_command_protocol_correlations correlation
		join device_commands command on command.id=correlation.command_id
		where correlation.project_id=$1 and correlation.adapter_id=$2 and correlation.transaction_id=$3
		  and correlation.business_id=$4 and correlation.method=$5
		for update of correlation,command`, event.ProjectID, envelope.AdapterID,
		reply.TransactionID, reply.BusinessID, reply.Method,
	).Scan(&commandID, &status, &commandKey, &liveStreamID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if status == "acknowledged" || status == "nacked" {
		return nil
	}
	if status == "unknown" {
		_, err := tx.ExecContext(ctx, `update device_command_protocol_correlations
			set reply_event_id=$2,reply_result=$3,reply_payload_json=$4,replied_at=$5,updated_at=now()
			where command_id=$1::uuid and status='unknown'`,
			commandID, envelope.EventID, reply.Result, payload.Data, dispatcher.now().UTC())
		return err
	}
	protocolStatus, commandStatus, outcome := "nacked", "nacked", "nack"
	if reply.Outcome() == ReplyAcknowledged {
		protocolStatus, commandStatus, outcome = "acknowledged", "acknowledged", "ack"
	}
	now := dispatcher.now().UTC()
	if _, err := tx.ExecContext(ctx, `
		update device_command_protocol_correlations
		set status=$2,reply_event_id=$3,reply_result=$4,reply_payload_json=$5,replied_at=$6,updated_at=now()
		where command_id=$1::uuid and status in ('prepared','sent')`,
		commandID, protocolStatus, envelope.EventID, reply.Result, payload.Data, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		update device_commands set status=$3,completed_at=$4,result_json=result_json||$5
		where project_id=$1 and id=$2::uuid and status in ('dispatchable','sent')`,
		event.ProjectID, commandID, commandStatus, now,
		jsonObject(map[string]any{"djiResult": reply.Result, "djiReplyEventId": envelope.EventID})); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		update command_attempts set status=$3,acknowledged_at=$4,error_code=case when $3='nacked' then $5 else null end,
		  result_json=$6
		where project_id=$1 and command_id=$2::uuid
		  and attempt=(select max(candidate.attempt) from command_attempts candidate where candidate.command_id=$2::uuid)`,
		event.ProjectID, commandID, commandStatus, now, fmt.Sprintf("DJI_RESULT_%d", reply.Result), payload.Data); err != nil {
		return err
	}
	if liveStreamID.Valid && (commandKey == "start" || commandKey == "stop") {
		streamStatus, reason := "failed", fmt.Sprintf("DJI_LIVE_%s_RESULT_%d", strings.ToUpper(commandKey), reply.Result)
		if reply.Outcome() == ReplyAcknowledged {
			streamStatus, reason = "starting", ""
			if commandKey == "stop" {
				streamStatus = "stopped"
			}
		}
		if _, err := tx.ExecContext(ctx, `update live_streams set status=$3,status_reason=nullif($4,''),
			ended_at=case when $3 in ('failed','stopped') then $5::timestamptz else null end,
			playback_ref=case when $3 in ('failed','stopped') then null else playback_ref end,
			lease_owner=case when $3 in ('failed','stopped') then null else lease_owner end,
			lease_expires_at=case when $3 in ('failed','stopped') then null else $5::timestamptz+interval '45 seconds' end,
			updated_at=now() where project_id=$1 and id=$2`,
			event.ProjectID, liveStreamID.Int64, streamStatus, reason, now); err != nil {
			return err
		}
		streamEventID := fmt.Sprintf("live-stream:%d:%s:%s", liveStreamID.Int64, commandKey, reply.TransactionID)
		if _, err := tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json)
			values($1,$2,$3,'live_stream.status_changed',$4) on conflict(event_id) do nothing`,
			event.ProjectID, event.TeamID, streamEventID,
			jsonObject(map[string]any{"streamId": liveStreamID.Int64, "status": streamStatus, "reason": reason})); err != nil {
			return err
		}
	}
	ackPayload := jsonObject(map[string]any{"commandId": commandID, "outcome": outcome})
	_, err = tx.ExecContext(ctx, `
		insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
		values ($1,$2,$3,'command.ack',$4) on conflict(event_id) do nothing`,
		event.ProjectID, event.TeamID, "command.ack:"+commandID+":"+reply.TransactionID, ackPayload)
	return err
}

func (dispatcher *CommandDispatcher) ExpireUnknown(ctx context.Context, database *sql.DB) (int64, error) {
	now := dispatcher.now().UTC()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		select command.id::text,command.project_id,command.team_id
		from device_commands command
		join device_command_protocol_correlations correlation on correlation.command_id=command.id
		where command.status='sent' and correlation.status='sent' and command.deadline_at <= $1
		order by command.deadline_at for update of command,correlation skip locked`, now)
	if err != nil {
		return 0, err
	}
	type expiredCommand struct {
		ID        string
		ProjectID int
		TeamID    int
	}
	var expired []expiredCommand
	for rows.Next() {
		var command expiredCommand
		if err := rows.Scan(&command.ID, &command.ProjectID, &command.TeamID); err != nil {
			rows.Close()
			return 0, err
		}
		expired = append(expired, command)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, command := range expired {
		if _, err := tx.ExecContext(ctx, `update device_commands
			set status='unknown',completed_at=$2,result_json=result_json||'{"reason":"DJI_REPLY_TIMEOUT"}'::jsonb
			where id=$1::uuid and status='sent'`, command.ID, now); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `update device_command_protocol_correlations
			set status='unknown',updated_at=now() where command_id=$1::uuid and status='sent'`, command.ID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `update command_attempts set status='timed_out',error_code='DJI_REPLY_TIMEOUT'
			where command_id=$1::uuid and status='sent'`, command.ID); err != nil {
			return 0, err
		}
		payload := jsonObject(map[string]any{"commandId": command.ID, "outcome": "timeout"})
		if _, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
			values($1,$2,$3,'command.ack',$4) on conflict(event_id) do nothing`,
			command.ProjectID, command.TeamID, "command.ack:"+command.ID+":timeout", payload); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(expired)), nil
}

func (dispatcher *CommandDispatcher) RunTimeoutReconciler(ctx context.Context, database *sql.DB, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := dispatcher.ExpireUnknown(ctx, database); err != nil && ctx.Err() == nil {
				return err
			}
		}
	}
}
