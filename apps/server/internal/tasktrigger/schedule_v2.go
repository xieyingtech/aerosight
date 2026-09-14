package tasktrigger

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AuthorizeDelegate rechecks and locks the explicit execution identity.
func AuthorizeDelegate(ctx context.Context, tx *sql.Tx, projectID int, versionID int64, uid int32) error {
	return authorizeDelegate(ctx, tx, candidate{ProjectID: projectID, TaskVersionID: versionID}, uid)
}

func authorizeDelegate(ctx context.Context, tx *sql.Tx, item candidate, uid int32) error {
	q := sqlcgen.New(tx)
	member, err := q.LockProjectMembership(ctx, sqlcgen.LockProjectMembershipParams{UserID: uid, ProjectID: int32(item.ProjectID)})
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("TASK_TRIGGER_DELEGATE_ACCESS_DENIED")
	}
	if err != nil {
		return err
	}
	grants, err := q.LockProjectPermissions(ctx, sqlcgen.LockProjectPermissionsParams{ProjectID: int32(item.ProjectID), TeamID: member.TeamID, UserID: uid})
	if err != nil {
		return err
	}
	required := map[string]bool{"mission:operate": true}
	rows, err := tx.QueryContext(ctx, "select uses from task_steps where project_id=$1 and task_version_id=$2", item.ProjectID, item.TaskVersionID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var uses string
		if err = rows.Scan(&uses); err != nil {
			rows.Close()
			return err
		}
		if uses == "copilot.run" {
			required["agent:use"] = true
		}
		if uses == "issue.create-or-update" {
			required["issue:handle"] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if member.Role == "owner" || member.Role == "admin" {
		return nil
	}
	for _, grant := range grants {
		delete(required, grant)
		if grant == "event:handle" {
			delete(required, "issue:handle")
		}
	}
	if len(required) > 0 {
		return errors.New("TASK_TRIGGER_DELEGATE_ACCESS_DENIED")
	}
	return nil
}

func recordSchedule(ctx context.Context, tx *sql.Tx, item candidate, key string, start, end *time.Time, outcome, reason string, run int) error {
	var runID any
	if run > 0 {
		runID = run
	}
	_, err := tx.ExecContext(ctx, `insert into task_trigger_records(project_id,team_id,task_id,task_version_id,occurrence_key,scheduled_for,interval_end,outcome,reason,task_run_id)
 values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) on conflict(project_id,task_id,occurrence_key) do nothing`, item.ProjectID, item.TeamID, item.TaskID, item.TaskVersionID, key, start, end, outcome, reason, runID)
	return err
}

func (scheduler *Scheduler) reconcileSchedule(ctx context.Context, item candidate) (int, error) {
	now := scheduler.now().UTC()
	tx, err := scheduler.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "select pg_advisory_xact_lock($1,$2)", item.ProjectID, item.TaskID); err != nil {
		return 0, err
	}
	var cursor sql.NullTime
	var triggerJSON []byte
	var version int64
	var status string
	err = tx.QueryRowContext(ctx, `select task.status,task.current_published_version_id,task.schedule_evaluated_at,version.trigger_json
 from tasks task join task_versions version on version.id=task.current_published_version_id and version.project_id=task.project_id
 where task.project_id=$1 and task.id=$2 for update of task,version`, item.ProjectID, item.TaskID).Scan(&status, &version, &cursor, &triggerJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if status != "active" || version != item.TaskVersionID {
		return 0, nil
	}
	if cursor.Valid && now.Before(cursor.Time) {
		return 0, nil
	}
	finish := func(count int) (int, error) {
		if _, err := tx.ExecContext(ctx, "update tasks set schedule_evaluated_at=$3 where project_id=$1 and id=$2", item.ProjectID, item.TaskID, now); err != nil {
			return 0, err
		}
		return count, tx.Commit()
	}
	trigger := scheduleTrigger{Enabled: true}
	parseErr := json.Unmarshal(triggerJSON, &trigger)
	var location *time.Location
	if parseErr == nil {
		location, parseErr = time.LoadLocation(trigger.Timezone)
	}
	if parseErr == nil {
		_, parseErr = CronMatches(trigger.Cron, now.In(location))
	}
	if parseErr != nil {
		key := fmt.Sprintf("schedule-error:%d:%s", version, now.Truncate(time.Minute).Format(time.RFC3339))
		if err = recordSchedule(ctx, tx, item, key, &now, nil, "error", "TASK_SCHEDULE_INVALID", 0); err != nil {
			return 0, err
		}
		return finish(0)
	}
	if !trigger.Enabled || trigger.Type != "schedule" {
		return finish(0)
	}
	// One interval audit represents unprocessed downtime, without catch-up flights.
	deadline := now.Add(-60 * time.Second)
	if cursor.Valid && cursor.Time.Before(deadline) {
		start := cursor.Time.UTC()
		end := deadline
		key := "schedule-missed:" + start.Format(time.RFC3339Nano)
		if err = recordSchedule(ctx, tx, item, key, &start, &end, "missed", "scheduler-unprocessed-window:no-catch-up", 0); err != nil {
			return 0, err
		}
	}
	first := now.Truncate(time.Minute)
	previous := first.Add(-time.Minute)
	if cursor.Valid && !previous.Before(deadline) && previous.After(cursor.Time) {
		first = previous
	}
	created := 0
	for occurrence := first; !occurrence.After(now); occurrence = occurrence.Add(time.Minute) {
		if cursor.Valid && !occurrence.After(cursor.Time) {
			continue
		}
		matched, err := CronMatches(trigger.Cron, occurrence.In(location))
		if err != nil {
			return 0, err
		}
		if !matched {
			continue
		}
		key := "schedule:" + occurrence.Format(time.RFC3339)
		var recorded bool
		if err = tx.QueryRowContext(ctx, "select exists(select 1 from task_trigger_records where project_id=$1 and task_id=$2 and occurrence_key=$3)", item.ProjectID, item.TaskID, key).Scan(&recorded); err != nil {
			return 0, err
		}
		if recorded {
			continue
		}
		snapshot := map[string]any{"trigger": map[string]any{"type": "schedule", "idempotencyKey": occurrence.Format(time.RFC3339), "occurredAt": now.Format(time.RFC3339Nano), "scheduledFor": occurrence.Format(time.RFC3339), "actor": map[string]any{"type": "service", "id": "task-scheduler"}}, "inputs": map[string]any{}}
		if _, err = tx.ExecContext(ctx, "savepoint schedule_occurrence"); err != nil {
			return 0, err
		}
		run, inserted, createErr := createRun(ctx, tx, item, "schedule", key, snapshot)
		outcome, reason := "skipped", "task-disabled-stale-or-concurrency-limit"
		if createErr != nil {
			if _, err = tx.ExecContext(ctx, "rollback to savepoint schedule_occurrence"); err != nil {
				return 0, err
			}
			outcome, reason = "error", createErr.Error()
			run = 0
		} else if run > 0 {
			outcome, reason = "accepted", "trigger-accepted"
			if inserted {
				created++
			}
		}
		if _, err = tx.ExecContext(ctx, "release savepoint schedule_occurrence"); err != nil {
			return 0, err
		}
		if err = recordSchedule(ctx, tx, item, key, &occurrence, nil, outcome, reason, run); err != nil {
			return 0, err
		}
	}
	return finish(created)
}
