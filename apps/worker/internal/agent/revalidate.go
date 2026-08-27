package agent

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type JobAuthorization struct {
	Context QueuedContext
	Current CurrentAuthorization
}

func RevalidateQueuedJob(ctx context.Context, tx *sql.Tx, jobID string, now time.Time) error {
	var value JobAuthorization
	err := tx.QueryRowContext(ctx, `
		select job.requested_by_user_id,job.team_id,job.project_id,job.session_id,job.required_permission,job.context_expires_at,
		       membership.user_id is not null,coalesce(membership.role,''),
		       membership.role in('owner','admin') or job.required_permission='project:view' or permission.permission is not null,
		       project.team_id,session.project_id,session.started_by_user_id,session.status='open'
		  from agent_tool_jobs job
		  join projects project on project.id=job.project_id
		  join agent_sessions session on session.id=job.session_id
		  left join team_members membership on membership.team_id=project.team_id and membership.user_id=job.requested_by_user_id
		  left join project_permissions permission on permission.project_id=job.project_id and permission.team_id=project.team_id
		       and permission.user_id=job.requested_by_user_id and permission.permission=job.required_permission
		 where job.id=$1 and job.status='queued' for update of job`, jobID).Scan(
		&value.Context.UserID, &value.Context.TeamID, &value.Context.ProjectID, &value.Context.SessionID,
		&value.Context.RequiredPermission, &value.Context.ExpiresAt, &value.Current.MembershipActive,
		&value.Current.TeamRole, &value.Current.PermissionGranted, &value.Current.ProjectTeamID,
		&value.Current.SessionProjectID, &value.Current.SessionUserID, &value.Current.SessionOpen,
	)
	if err != nil {
		return err
	}
	value.Current.Now = now
	if err := AuthorizeQueuedExecution(value.Context, value.Current); err != nil {
		if !errors.Is(err, ErrQueuedAuthorizationDenied) {
			return err
		}
		_, updateErr := tx.ExecContext(ctx, `update agent_tool_jobs set status='failed',failure_code='AUTHORIZATION_REVALIDATION_FAILED',
			authorization_checked_at=$2,finished_at=$2 where id=$1 and status='queued'`, jobID, now)
		return updateErr
	}
	result, err := tx.ExecContext(ctx, `update agent_tool_jobs set status='running',authorization_checked_at=$2,started_at=$2
		where id=$1 and status='queued'`, jobID, now)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return sql.ErrNoRows
	}
	return nil
}
