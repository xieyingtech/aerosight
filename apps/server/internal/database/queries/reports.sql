-- name: LockReportPublication :one
SELECT id FROM generated_reports WHERE project_id=$1 AND id=$2 FOR UPDATE;

-- name: LockReportDraft :one
SELECT id,completeness,content_json FROM generated_report_versions WHERE project_id=$1 AND generated_report_id=$2 AND status='draft' FOR UPDATE;

-- name: RetirePublishedReport :exec
UPDATE generated_report_versions SET status='retired' WHERE project_id=$1 AND generated_report_id=$2 AND status='published';

-- name: PublishReportVersion :exec
UPDATE generated_report_versions SET status='published',published_by_user_id=$3,published_at=now() WHERE project_id=$1 AND id=$2;

-- name: PointPublishedReport :exec
UPDATE generated_reports SET current_published_version_id=$3,updated_at=now() WHERE project_id=$1 AND id=$2;

-- name: RetainReportAssets :exec
UPDATE assets SET legal_hold=true,retention_reason='published-report:'||sqlc.arg(report_id)::text
WHERE project_id=$1 AND id=ANY(sqlc.arg(asset_ids)::int[]);

-- name: ExportPublishedReport :one
SELECT to_jsonb(r) FROM (
 SELECT report.id::text,report.title,version.version,version.completeness,version.content_json AS content,
 version.data_gaps_json AS "dataGaps",version.published_at AS "publishedAt"
 FROM generated_reports report JOIN generated_report_versions version ON version.id=report.current_published_version_id AND version.project_id=report.project_id
 WHERE report.project_id=$1 AND report.id=$2 AND version.status='published'
) r;
