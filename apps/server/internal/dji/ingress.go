package dji

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type IngressDisposition string

const (
	IngressAccepted   IngressDisposition = "accepted"
	IngressDuplicate  IngressDisposition = "duplicate"
	IngressOutOfOrder IngressDisposition = "out_of_order"
)

type IngressStore interface {
	Accept(context.Context, RoutedMessage) (IngressDisposition, error)
}

type PermanentIngressError struct{ Cause error }

func (failure PermanentIngressError) Error() string { return failure.Cause.Error() }
func (failure PermanentIngressError) Unwrap() error { return failure.Cause }

func IsPermanentIngressError(err error) bool {
	var failure PermanentIngressError
	return errors.As(err, &failure)
}

type SQLIngressStore struct{ db *sql.DB }

func NewSQLIngressStore(db *sql.DB) *SQLIngressStore { return &SQLIngressStore{db: db} }

func (store *SQLIngressStore) Accept(ctx context.Context, message RoutedMessage) (IngressDisposition, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var teamID int
	if err := tx.QueryRowContext(ctx,
		"select team_id from device_adapters where id = $1 and project_id = $2",
		message.Envelope.AdapterID, message.Envelope.ProjectID,
	).Scan(&teamID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("DJI_INGRESS_SCOPE_MISMATCH")
		}
		return "", err
	}
	var messageID int64
	err = tx.QueryRowContext(ctx, `
		insert into device_protocol_messages (
		  project_id, team_id, adapter_id, gateway_sn, device_sn, topic, route_kind,
		  transaction_id, business_id, method, timestamp_ms, sequence_number,
		  qos, duplicate_flag, payload_json
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		on conflict (adapter_id, topic, transaction_id) do nothing
		returning id`,
		message.Envelope.ProjectID, teamID, message.Envelope.AdapterID, message.GatewaySN,
		message.DeviceSN, message.Topic, message.Kind, message.TransactionID, emptyToNil(message.BusinessID),
		emptyToNil(message.Method), message.TimestampMS, message.Sequence, message.QoS, message.Duplicate,
		message.RawPayload,
	).Scan(&messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return IngressDuplicate, tx.Commit()
	}
	if err != nil {
		return "", err
	}
	if message.Ordered() {
		var routeKey string
		err = tx.QueryRowContext(ctx, `
			insert into device_protocol_cursors (
			  project_id, team_id, adapter_id, route_key, last_timestamp_ms, last_transaction_id
			) values ($1,$2,$3,$4,$5,$6)
			on conflict (adapter_id, route_key) do update
			set last_timestamp_ms = excluded.last_timestamp_ms,
			    last_transaction_id = excluded.last_transaction_id, updated_at = now()
			where excluded.last_timestamp_ms >= device_protocol_cursors.last_timestamp_ms
			returning route_key`,
			message.Envelope.ProjectID, teamID, message.Envelope.AdapterID, message.RouteKey(),
			message.TimestampMS, message.TransactionID,
		).Scan(&routeKey)
		if errors.Is(err, sql.ErrNoRows) {
			_, updateErr := tx.ExecContext(ctx, `
				update device_protocol_messages
				set disposition = 'out_of_order', disposition_reason = 'DJI_MESSAGE_TIMESTAMP_REGRESSION'
				where id = $1`, messageID)
			if updateErr != nil {
				return "", updateErr
			}
			return IngressOutOfOrder, tx.Commit()
		}
		if err != nil {
			return "", err
		}
	}
	envelopeJSON, err := json.Marshal(message.Envelope)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		insert into outbox_events (project_id, team_id, event_id, event_type, payload_json)
		values ($1,$2,$3,$4,$5)
		on conflict (event_id) do nothing`,
		message.Envelope.ProjectID, teamID, message.Envelope.EventID, message.Envelope.EventType, envelopeJSON,
	); err != nil {
		return "", err
	}
	return IngressAccepted, tx.Commit()
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

type MessageIngestor struct{ store IngressStore }

func NewMessageIngestor(store IngressStore) *MessageIngestor { return &MessageIngestor{store: store} }

func (ingestor *MessageIngestor) Handle(scope RouteContext) MQTTMessageHandler {
	return func(ctx context.Context, mqttMessage MQTTMessage) error {
		routed, err := RouteMQTTMessage(scope, mqttMessage)
		if err != nil {
			return PermanentIngressError{Cause: err}
		}
		if err := routed.Envelope.ValidateForScope(scope.ProjectID, scope.AdapterID); err != nil {
			return PermanentIngressError{Cause: err}
		}
		_, err = ingestor.store.Accept(ctx, routed)
		return err
	}
}
