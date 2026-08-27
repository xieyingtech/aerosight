package dji

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aerosight/worker/internal/adapter"
	"aerosight/worker/internal/outbox"
)

type CommandPublisher interface {
	Publish(context.Context, int64, string, []byte) error
}

type CommandDispatcher struct {
	publisher CommandPublisher
	now       func() time.Time
}

func NewCommandDispatcher(publisher CommandPublisher, now func() time.Time) (*CommandDispatcher, error) {
	if publisher == nil {
		return nil, errors.New("DJI_COMMAND_PUBLISHER_REQUIRED")
	}
	if now == nil {
		now = time.Now
	}
	return &CommandDispatcher{publisher: publisher, now: now}, nil
}

func (dispatcher *CommandDispatcher) DispatchHandler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var request struct {
		CommandID string `json:"commandId"`
	}
	if json.Unmarshal(event.Payload, &request) != nil || request.CommandID == "" {
		return errors.New("DJI_COMMAND_EVENT_INVALID")
	}
	var command struct {
		ID             string
		CapabilityCode string
		CommandKey     string
		Parameters     json.RawMessage
		Deadline       time.Time
		AdapterID      int64
		GatewaySN      string
	}
	err := tx.QueryRowContext(ctx, `
		select command.id::text,command.capability_code,command.command_key,command.parameters_json,
		       command.deadline_at,device.adapter_id,
		       coalesce(gateway_identity.external_device_id,target_identity.external_device_id)
		from device_commands command
		join devices device on device.id=command.device_id and device.project_id=command.project_id
		join device_types device_type on device_type.id=device.device_type_id
		join driver_definitions driver on driver.id=device_type.driver_definition_id and driver.driver_key='dji.cloud'
		join device_external_identities target_identity
		  on target_identity.device_id=device.id and target_identity.adapter_id=device.adapter_id
		left join lateral (
		  select relation.from_device_id from device_relationships relation
		  where relation.project_id=command.project_id and relation.to_device_id=device.id
		    and relation.relation_type='docked-aircraft' and relation.valid_until is null
		  order by relation.valid_from desc limit 1
		) gateway on true
		left join device_external_identities gateway_identity
		  on gateway_identity.device_id=gateway.from_device_id and gateway_identity.adapter_id=device.adapter_id
		where command.project_id=$1 and command.id=$2::uuid and command.status='dispatchable'
		for update of command`, event.ProjectID, request.CommandID).Scan(
		&command.ID, &command.CapabilityCode, &command.CommandKey, &command.Parameters,
		&command.Deadline, &command.AdapterID, &command.GatewaySN,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	now := dispatcher.now().UTC()
	if !now.Before(command.Deadline) {
		_, err := tx.ExecContext(ctx, `update device_commands set status='timed_out',completed_at=$3
			where project_id=$1 and id=$2::uuid and status='dispatchable'`, event.ProjectID, command.ID, now)
		return err
	}
	service, err := BuildServiceCommand(command.GatewaySN, command.ID, command.ID,
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
		service.Topic, service.Payload).Scan(&correlationStatus)
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
	var commandID, status string
	err = tx.QueryRowContext(ctx, `
		select command_id::text,status from device_command_protocol_correlations
		where project_id=$1 and adapter_id=$2 and transaction_id=$3 and business_id=$4 and method=$5
		for update`, event.ProjectID, envelope.AdapterID, reply.TransactionID, reply.BusinessID, reply.Method,
	).Scan(&commandID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if status == "acknowledged" || status == "nacked" {
		return nil
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
	ackPayload := jsonObject(map[string]any{"commandId": commandID, "outcome": outcome})
	_, err = tx.ExecContext(ctx, `
		insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
		values ($1,$2,$3,'command.ack',$4) on conflict(event_id) do nothing`,
		event.ProjectID, event.TeamID, "command.ack:"+commandID+":"+reply.TransactionID, ackPayload)
	return err
}
