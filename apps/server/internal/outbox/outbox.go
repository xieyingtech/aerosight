package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"
)

type Event struct {
	ID          int64
	ProjectID   int
	TeamID      int
	EventID     string
	EventType   string
	Payload     json.RawMessage
	Attempts    int
	MaxAttempts int
}

type Handler func(context.Context, *sql.Tx, Event) error

type Repository interface {
	Claim(context.Context, string, []string, int, time.Duration) ([]Event, error)
	Process(context.Context, string, Event, Handler) error
	Complete(context.Context, string, int64) error
	Fail(context.Context, string, Event, error, time.Duration) (string, error)
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (store *Store) Claim(ctx context.Context, workerID string, eventTypes []string, limit int, lease time.Duration) ([]Event, error) {
	rows, err := store.db.QueryContext(ctx, `
		with candidates as (
			select id
			from outbox_events
			where attempts < max_attempts
			  and event_type = any($1::text[])
			  and available_at <= now()
			  and (status = 'pending' or (status = 'processing' and locked_until < now()))
			order by available_at, id
			for update skip locked
			limit $2
		)
		update outbox_events event
		set status = 'processing', attempts = event.attempts + 1,
		    locked_by = $3, locked_until = now() + ($4 * interval '1 millisecond'),
		    last_error = null
		from candidates
		where event.id = candidates.id
		returning event.id, event.project_id, event.team_id, event.event_id,
		          event.event_type, event.payload_json, event.attempts, event.max_attempts`,
		eventTypes, limit, workerID, lease.Milliseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(
			&event.ID, &event.ProjectID, &event.TeamID, &event.EventID,
			&event.EventType, &event.Payload, &event.Attempts, &event.MaxAttempts,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (store *Store) Process(ctx context.Context, consumerName string, event Event, handler Handler) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var consumed bool
	if err := tx.QueryRowContext(ctx,
		"select exists(select 1 from outbox_consumptions where consumer_name = $1 and event_id = $2)",
		consumerName, event.EventID,
	).Scan(&consumed); err != nil {
		return err
	}
	if consumed {
		return tx.Commit()
	}
	if err := handler(ctx, tx, event); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"insert into outbox_consumptions (consumer_name, event_id) values ($1, $2)",
		consumerName, event.EventID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *Store) Complete(ctx context.Context, workerID string, eventID int64) error {
	result, err := store.db.ExecContext(ctx, `
		update outbox_events
		set status = 'completed', locked_by = null, locked_until = null, completed_at = now()
		where id = $1 and status = 'processing' and locked_by = $2`, eventID, workerID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("outbox completion lost its lease")
	}
	return nil
}

func (store *Store) Fail(
	ctx context.Context,
	workerID string,
	event Event,
	cause error,
	retryAfter time.Duration,
) (string, error) {
	var status string
	err := store.db.QueryRowContext(ctx, `
		update outbox_events
		set status = case when attempts >= max_attempts then 'dead' else 'pending' end,
		    available_at = case when attempts >= max_attempts then available_at
		                        else now() + ($4 * interval '1 millisecond') end,
		    locked_by = null, locked_until = null, last_error = left($3, 2000)
		where id = $1 and status = 'processing' and locked_by = $2
		returning status`, event.ID, workerID, cause.Error(), retryAfter.Milliseconds()).Scan(&status)
	return status, err
}

type Consumer struct {
	repository Repository
	workerID   string
	name       string
	handlers   map[string]Handler
	logger     *slog.Logger
	batchSize  int
	lease      time.Duration
	poll       time.Duration
}

func NewConsumer(repository Repository, workerID, name string, logger *slog.Logger) *Consumer {
	return &Consumer{
		repository: repository,
		workerID:   workerID,
		name:       name,
		handlers:   map[string]Handler{},
		logger:     logger,
		batchSize:  20,
		lease:      30 * time.Second,
		poll:       time.Second,
	}
}

func (consumer *Consumer) Register(eventType string, handler Handler) {
	consumer.handlers[eventType] = handler
}

func (consumer *Consumer) ConsumeOnce(ctx context.Context) (int, error) {
	eventTypes := make([]string, 0, len(consumer.handlers))
	for eventType := range consumer.handlers {
		eventTypes = append(eventTypes, eventType)
	}
	slices.Sort(eventTypes)
	events, err := consumer.repository.Claim(ctx, consumer.workerID, eventTypes, consumer.batchSize, consumer.lease)
	if err != nil {
		return 0, err
	}
	for _, event := range events {
		handler, ok := consumer.handlers[event.EventType]
		if !ok {
			return 0, fmt.Errorf("repository claimed unregistered event type %q", event.EventType)
		}
		if err := consumer.repository.Process(ctx, consumer.name, event, handler); err != nil {
			retryAfter := retryDelay(event.Attempts)
			status, failErr := consumer.repository.Fail(ctx, consumer.workerID, event, err, retryAfter)
			consumer.logger.Error("outbox event failed",
				"event_id", event.EventID, "event_type", event.EventType,
				"attempt", event.Attempts, "status", status, "error", err.Error())
			if failErr != nil {
				return 0, errors.Join(err, failErr)
			}
			continue
		}
		if err := consumer.repository.Complete(ctx, consumer.workerID, event.ID); err != nil {
			return 0, err
		}
	}
	return len(events), nil
}

func (consumer *Consumer) Run(ctx context.Context) error {
	return consumer.RunWithWake(ctx, nil)
}

func (consumer *Consumer) RunWithWake(ctx context.Context, wake <-chan struct{}) error {
	ticker := time.NewTicker(consumer.poll)
	defer ticker.Stop()
	for {
		if _, err := consumer.ConsumeOnce(ctx); err != nil {
			consumer.logger.Error("outbox poll failed", "error", err.Error())
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case _, ok := <-wake:
			if !ok {
				wake = nil
			}
		}
	}
}

func retryDelay(attempt int) time.Duration {
	seconds := math.Min(300, math.Pow(2, float64(max(0, attempt-1))))
	return time.Duration(seconds) * time.Second
}
