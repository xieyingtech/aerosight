-- name: FeedbackReplay :many
SELECT to_jsonb(r) FROM (
select id,action,created_at as "createdAt" from issue_feedback
      where project_id=sqlc.arg(p1) and issue_id=sqlc.arg(p2) and client_key=sqlc.arg(p3)
) r;

-- name: FeedbackEvidence :many
SELECT to_jsonb(r) FROM (
select issue.project_id as "issueProjectId",issue.state_version as "issueVersion",detection.id::int as "detectionId",
      detection.label as "originalLabel",run.algorithm_definition_version_id::int as "algorithmDefinitionVersionId",
      issue.task_version_id::int as "taskVersionId",(
        select case when link.target_id~'^[0-9]+$' then link.target_id::bigint end from issue_links link
        where link.project_id=issue.project_id and link.issue_id=issue.id and link.link_type='task_step' order by link.id desc limit 1
      )::int as "taskRunStepId"
      from issues issue join issue_links detection_link on detection_link.project_id=issue.project_id
        and detection_link.issue_id=issue.id and detection_link.link_type='detection'
      join detections detection on detection.project_id=detection_link.project_id
        and detection.id=case when detection_link.target_id~'^[0-9]+$' then detection_link.target_id::bigint end
      join algorithm_runs run on run.id=detection.algorithm_run_id and run.project_id=detection.project_id
      where issue.project_id=sqlc.arg(p1) and issue.id=sqlc.arg(p2) and detection.id=sqlc.arg(p3) for update of issue
) r;

-- name: FeedbackInsert :one
insert into issue_feedback(
      project_id,team_id,issue_id,detection_id,algorithm_definition_version_id,task_version_id,task_run_step_id,
      action,corrected_label,disposition,reason,client_key,evidence_snapshot_json,actor_user_id)
      values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),sqlc.arg(p4),sqlc.arg(p5),sqlc.arg(p6),sqlc.arg(p7),sqlc.arg(p8),sqlc.arg(p9),sqlc.arg(p10),sqlc.arg(p11),sqlc.arg(p12),sqlc.arg(p13),sqlc.arg(p14)) returning id,created_at as "createdAt";

-- name: FeedbackEvent :exec
insert into issue_events(project_id,issue_id,event_type,body,metadata_json,actor_user_id,client_key)
      values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),sqlc.arg(p4),sqlc.arg(p5),sqlc.arg(p6),sqlc.arg(p7));

-- name: FeedbackUpdate :exec
update issues set state_version=state_version+1,updated_at=now() where project_id=sqlc.arg(p1) and id=sqlc.arg(p2);
