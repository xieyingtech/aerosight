package algorithm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"aerosight/server/internal/outbox"
	"aerosight/server/internal/tasktrigger"
)

// WithInspectionWorker moves inspection child HTTP work out of outbox transactions.
// Legacy algorithm handlers retain their existing dispatch path.
func (p *Processor) WithInspectionWorker(db *sql.DB) *Processor { p.inspectionDB = db; return p }

type bufferedAttempts struct{ values []Attempt }

func (b *bufferedAttempts) RecordAttempt(_ context.Context, a Attempt) error {
	b.values = append(b.values, a)
	return nil
}

func (p *Processor) RunInspection(ctx context.Context, interval time.Duration) error {
	if p.inspectionDB == nil {
		return errors.New("inspection algorithm database is required")
	}
	return runInspectionLoop(ctx, interval, p.ProcessInspectionNext)
}

func runInspectionLoop(ctx context.Context, interval time.Duration, next func(context.Context) (bool, error)) error {
	if interval <= 0 {
		return errors.New("inspection algorithm interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		if _, err := next(ctx); err != nil && ctx.Err() == nil {
			// Provider failures are persisted by ProcessInspectionNext. A failed
			// database transaction leaves the queued job or lease recoverable;
			// retry on the next tick instead of stopping peer runtime services.
			slog.Warn("inspection algorithm worker will retry", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ProcessInspectionNext uses started_at as a two-minute lease for a bounded
// one-minute read-only request. Only the current lease may commit its response.
func (p *Processor) ProcessInspectionNext(ctx context.Context) (bool, error) {
	if p.inspectionDB == nil {
		return false, errors.New("inspection algorithm database is required")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	tx, err := p.inspectionDB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var runID, childState string
	var businessID int64
	var event outbox.Event
	err = tx.QueryRowContext(ctx, `select child.id::text,child.status,child.project_id,child.team_id,business.id
 from algorithm_runs child join task_run_steps rs on rs.id=child.task_run_step_id and rs.project_id=child.project_id
 join task_steps step on step.id=rs.task_step_id and step.uses='inspection.detect'
 join task_runs business on business.id=child.task_run_id and business.id=rs.task_run_id and business.project_id=child.project_id
 where (child.status='queued' or (child.status='running' and child.started_at<now()-interval '2 minutes')) and business.status<>'paused'
 order by child.created_at,child.id for update of business skip locked limit 1`).Scan(&runID, &childState, &event.ProjectID, &event.TeamID, &businessID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if childState == "running" {
		updated, updateErr := tx.ExecContext(ctx, "update algorithm_runs set status='queued' where id=$1 and status='running' and started_at<now()-interval '2 minutes'", runID)
		if updateErr != nil {
			return false, updateErr
		}
		count, countErr := updated.RowsAffected()
		if countErr != nil {
			return false, countErr
		}
		if count != 1 {
			return false, nil
		}
	}
	event.Payload, _ = json.Marshal(map[string]string{"runId": runID})
	prepared, err := p.prepare(ctx, tx, event)
	if err != nil {
		return false, err
	}
	if prepared == nil {
		return true, tx.Commit()
	}
	var lease time.Time
	if err = tx.QueryRowContext(ctx, "update algorithm_runs set started_at=clock_timestamp() where id=$1 returning started_at", runID).Scan(&lease); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	attempts := &bufferedAttempts{}
	outcome, executeErr := NewHTTPJSONAdapter(p.client, attempts, p.breaker).Execute(ctx, prepared.request)
	// A request deadline must not prevent recording its timeout result.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer finishCancel()
	tx, err = p.inspectionDB.BeginTx(finishCtx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var state string
	var user sql.NullInt32
	var version int64
	if err = tx.QueryRowContext(finishCtx, "select status,created_by_user_id,task_version_id from task_runs where project_id=$1 and id=$2 for update", event.ProjectID, businessID).Scan(&state, &user, &version); err != nil {
		return false, err
	}
	var currentLease time.Time
	if err = tx.QueryRowContext(finishCtx, "select status,started_at from algorithm_runs where id=$1 for update", runID).Scan(&childState, &currentLease); err != nil {
		return false, err
	}
	if childState != "running" || !currentLease.Equal(lease) {
		return true, nil
	}
	recorder := transactionRecorder{tx: tx, projectID: event.ProjectID, teamID: event.TeamID}
	for _, a := range attempts.values {
		if err = recorder.RecordAttempt(finishCtx, a); err != nil {
			return false, err
		}
	}
	if state != "running" && state != "dispatching" {
		err = p.finishFailed(finishCtx, tx, event.ProjectID, runID, "canceled", "INSPECTION_PARENT_NOT_ACTIVE", "parent no longer active", outcome)
	} else if !user.Valid {
		err = p.finishFailed(finishCtx, tx, event.ProjectID, runID, "failed", "INSPECTION_ALGORITHM_DELEGATE_DENIED", "delegate unavailable", outcome)
	} else if authErr := tasktrigger.AuthorizeDelegate(finishCtx, tx, event.ProjectID, version, user.Int32); authErr != nil {
		err = p.finishFailed(finishCtx, tx, event.ProjectID, runID, "failed", "INSPECTION_ALGORITHM_DELEGATE_DENIED", "delegate authorization changed", outcome)
	} else {
		err = p.finish(finishCtx, tx, prepared, outcome, executeErr)
	}
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}
