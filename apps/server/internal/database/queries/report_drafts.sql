-- name: ReadReportSources :one
WITH run AS (
 SELECT r.*,t.name AS task_name,v.version AS task_version,v.definition_json,d.name AS device_name,d.type AS device_type,d.updated_at AS device_updated_at
 FROM task_runs r JOIN tasks t ON t.id=r.task_id AND t.project_id=r.project_id
 LEFT JOIN task_versions v ON v.id=r.task_version_id AND v.project_id=r.project_id
 LEFT JOIN devices d ON d.id=r.selected_device_id AND d.project_id=r.project_id
 WHERE r.project_id=$1 AND r.id=$2
), events AS (
 SELECT DISTINCT e.id,e.status,e.state_version AS "stateVersion",e.title,e.last_detected_at AS "lastDetectedAt"
 FROM perception_events e JOIN detection_group_members m ON m.detection_group_id=e.detection_group_id AND m.project_id=e.project_id
 JOIN detections d ON d.id=m.detection_id AND d.project_id=m.project_id
 WHERE e.project_id=$1 AND d.task_run_id=$2
)
SELECT jsonb_build_object(
 'taskRun',(SELECT to_jsonb(x) FROM (SELECT id,status,state_version AS "stateVersion",started_at AS "startedAt",finished_at AS "finishedAt",task_version_id::text AS "taskVersionId",selected_device_id AS "deviceId",task_name AS "taskName",task_version AS "taskVersion",definition_json AS definition,device_name AS "deviceName",device_type AS "deviceType",device_updated_at AS "deviceUpdatedAt" FROM run)x),
 'steps',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.position) FROM (SELECT s.id::text,s.position,s.status,s.result_json AS result,s.finished_at AS "finishedAt" FROM task_run_steps s WHERE s.project_id=$1 AND s.task_run_id=$2)x),'[]'),
 'track',(SELECT to_jsonb(x) FROM (SELECT count(*)::int AS "pointCount",min(p.captured_at) AS "startedAt",max(p.captured_at) AS "endedAt",min(p.spatial_quality) AS quality FROM poses p,run WHERE p.project_id=$1 AND p.device_id=run.selected_device_id AND (run.started_at IS NULL OR p.captured_at>=run.started_at AT TIME ZONE 'UTC') AND (run.finished_at IS NULL OR p.captured_at<=run.finished_at AT TIME ZONE 'UTC') HAVING count(*)>0)x),
 'events',coalesce((SELECT jsonb_agg(to_jsonb(events)) FROM events),'[]'),
 'feedback',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x."createdAt") FROM (SELECT f.id::text,f.perception_event_id AS "eventId",f.action,f.reason,f.created_at AS "createdAt" FROM event_feedback f WHERE f.project_id=$1 AND f.perception_event_id IN(SELECT events.id FROM events))x),'[]'),
 'assets',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.created_at) FROM (SELECT a.id,a.version,a.checksum_sha256 AS checksum,a.captured_at AS "capturedAt",a.created_at FROM assets a WHERE a.project_id=$1 AND a.task_run_id=$2 AND a.status='available' AND a.deleted_at IS NULL)x),'[]')
) FROM run;

-- name: UpsertDraftReport :one
INSERT INTO generated_reports(project_id,team_id,source_type,source_id,title,created_by_user_id)
VALUES($1,$2,'task_run',$3,$4,$5) ON CONFLICT(project_id,source_type,source_id) DO UPDATE SET title=excluded.title,updated_at=now() RETURNING id;

-- name: RetireReportDrafts :exec
UPDATE generated_report_versions SET status='retired' WHERE project_id=$1 AND generated_report_id=$2 AND status='draft';

-- name: NextReportVersion :one
SELECT (coalesce(max(version),0)::int+1)::int AS version FROM generated_report_versions WHERE generated_report_id=$1;

-- name: InsertReportDraft :one
INSERT INTO generated_report_versions(project_id,team_id,generated_report_id,version,completeness,content_json,data_gaps_json,created_by_user_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id;

-- name: InsertReportEvidence :exec
INSERT INTO generated_report_evidence(project_id,report_version_id,evidence_type,evidence_id,evidence_version,asset_id,checksum_sha256,href)
VALUES($1,$2,$3,$4,$5,$6,$7,$8);
