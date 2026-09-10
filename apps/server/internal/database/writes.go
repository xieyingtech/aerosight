package database

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sqlc-dev/pqtype"
	"time"
)

type AuditContext struct {
	ProjectID, TeamID, ActorUserID, ActorAgentID                int32
	RequestID, IdempotencyKey, Action, ResourceType, ResourceID string
	Input                                                       any
	PolicyResult                                                map[string]any
}
type WriteTx struct {
	Tx      *sql.Tx
	Queries *sqlcgen.Queries
}
type Reauthorize func(context.Context, *WriteTx) error

func optionalString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
func optionalID(value int32) sql.NullInt32 { return sql.NullInt32{Int32: value, Valid: value > 0} }
func affectedOne(count int64, err error) error {
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("WRITE_RECORD_MISSING")
	}
	return nil
}

// AuditedWrite owns one transaction. Authorize runs before both audit and domain
// mutations, and callers pass this same WriteTx into idempotency and event writes.
func AuditedWrite[T any](ctx context.Context, db *sql.DB, a AuditContext, authorize Reauthorize, operation func(*WriteTx) (T, error)) (T, error) {
	var result T
	if (a.ActorUserID <= 0 && a.ActorAgentID <= 0) || authorize == nil {
		return result, errors.New("AUDIT_ACTOR_AND_AUTHORIZATION_REQUIRED")
	}
	err := InTx(ctx, db, func(tx *sql.Tx, q *sqlcgen.Queries) error {
		w := &WriteTx{Tx: tx, Queries: q}
		if err := authorize(ctx, w); err != nil {
			return err
		}
		inputHash, err := AuditHash(a.Input)
		if err != nil {
			return err
		}
		policy := a.PolicyResult
		if policy == nil {
			policy = map[string]any{}
		}
		raw, err := json.Marshal(policy)
		if err != nil {
			return err
		}
		id, err := q.InsertProjectAudit(ctx, sqlcgen.InsertProjectAuditParams{ProjectID: a.ProjectID, TeamID: a.TeamID, RequestID: a.RequestID, IdempotencyKey: optionalString(a.IdempotencyKey), ActorUserID: optionalID(a.ActorUserID), ActorAgentID: optionalID(a.ActorAgentID), Action: a.Action, ResourceType: a.ResourceType, ResourceID: optionalString(a.ResourceID), InputHash: inputHash, PolicyResult: raw})
		if err != nil {
			return err
		}
		result, err = operation(w)
		if err != nil {
			return err
		}
		resultHash, err := AuditHash(result)
		if err != nil {
			return err
		}
		return affectedOne(q.CompleteProjectAudit(ctx, sqlcgen.CompleteProjectAuditParams{ID: id, ProjectID: a.ProjectID, ResultHash: optionalString(resultHash)}))
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

func AuditedPlatformWrite[T any](ctx context.Context, db *sql.DB, a AuditContext, authorize Reauthorize, operation func(*WriteTx) (T, error)) (T, error) {
	var result T
	if a.ActorUserID <= 0 || authorize == nil {
		return result, errors.New("AUDIT_ACTOR_AND_AUTHORIZATION_REQUIRED")
	}
	err := InTx(ctx, db, func(tx *sql.Tx, q *sqlcgen.Queries) error {
		w := &WriteTx{Tx: tx, Queries: q}
		if err := authorize(ctx, w); err != nil {
			return err
		}
		hash, err := AuditHash(a.Input)
		if err != nil {
			return err
		}
		id, err := q.InsertPlatformAudit(ctx, sqlcgen.InsertPlatformAuditParams{ActorUserID: a.ActorUserID, RequestID: a.RequestID, Action: a.Action, ResourceType: a.ResourceType, ResourceID: optionalString(a.ResourceID), InputHash: hash})
		if err != nil {
			return err
		}
		result, err = operation(w)
		if err != nil {
			return err
		}
		hash, err = AuditHash(result)
		if err != nil {
			return err
		}
		return affectedOne(q.CompletePlatformAudit(ctx, sqlcgen.CompletePlatformAuditParams{ID: id, ResultHash: optionalString(hash)}))
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

type ProjectEvent struct {
	ProjectID, TeamID  int32
	EventID, EventType string
	Payload            any
	OccurredAt         time.Time
	NoEnqueue          bool
	MaxAttempts        int32
}

func (w *WriteTx) Publish(ctx context.Context, event ProjectEvent) (int64, error) {
	payload := event.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	cursor, err := w.Queries.PublishProjectEvent(ctx, sqlcgen.PublishProjectEventParams{ProjectID: event.ProjectID, TeamID: event.TeamID, EventID: event.EventID, EventType: event.EventType, PayloadJson: raw, OccurredAt: sql.NullTime{Time: event.OccurredAt, Valid: !event.OccurredAt.IsZero()}})
	if err != nil {
		return 0, err
	}
	if !event.NoEnqueue {
		attempts := event.MaxAttempts
		if attempts == 0 {
			attempts = 8
		}
		err = w.Queries.EnqueueProjectEvent(ctx, sqlcgen.EnqueueProjectEventParams{ProjectID: event.ProjectID, TeamID: event.TeamID, EventID: event.EventID, EventType: event.EventType, PayloadJson: raw, MaxAttempts: attempts})
	}
	return cursor, err
}

type IdempotencyContext struct {
	ProjectID, TeamID        int32
	ActorKey, Operation, Key string
	Request                  any
}
type IdempotentResult[T any] struct {
	Value    T
	Replayed bool
}

func Idempotent[T any](ctx context.Context, w *WriteTx, input IdempotencyContext, operation func() (T, error)) (IdempotentResult[T], error) {
	var out IdempotentResult[T]
	if input.Key == "" || input.ActorKey == "" {
		return out, errors.New("IDEMPOTENCY_KEY_REQUIRED")
	}
	hash, err := AuditHash(input.Request)
	if err != nil {
		return out, err
	}
	id, err := w.Queries.ReserveIdempotency(ctx, sqlcgen.ReserveIdempotencyParams{ProjectID: input.ProjectID, TeamID: input.TeamID, ActorKey: input.ActorKey, Operation: input.Operation, IdempotencyKey: input.Key, RequestHash: hash})
	if errors.Is(err, sql.ErrNoRows) {
		record, err := w.Queries.ReadIdempotency(ctx, sqlcgen.ReadIdempotencyParams{ProjectID: input.ProjectID, ActorKey: input.ActorKey, Operation: input.Operation, IdempotencyKey: input.Key})
		if err != nil {
			return out, err
		}
		if record.RequestHash != hash {
			matches, err := matchesAuditHash(input.Request, record.RequestHash)
			if err != nil {
				return out, err
			}
			if !matches {
				return out, errors.New("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST")
			}
		}
		if record.Status == "processing" {
			return out, errors.New("IDEMPOTENCY_OPERATION_IN_PROGRESS")
		}
		if record.Status == "failed" {
			code := record.ErrorCode.String
			if code == "" {
				code = "IDEMPOTENCY_OPERATION_FAILED"
			}
			return out, errors.New(code)
		}
		if !record.ResponseJson.Valid {
			return out, fmt.Errorf("IDEMPOTENCY_RESPONSE_MISSING")
		}
		if err = json.Unmarshal(record.ResponseJson.RawMessage, &out.Value); err != nil {
			return out, err
		}
		out.Replayed = true
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Value, err = operation()
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(out.Value)
	if err != nil {
		return out, err
	}
	err = affectedOne(w.Queries.CompleteIdempotency(ctx, sqlcgen.CompleteIdempotencyParams{ID: id, ResponseJson: pqtype.NullRawMessage{RawMessage: raw, Valid: true}}))
	return out, err
}
