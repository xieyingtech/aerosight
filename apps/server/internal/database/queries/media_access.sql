-- name: ReadMediaAccessAsset :one
SELECT asset.id,asset.project_id,asset.storage_key,asset.mime_type,asset.kind,intent.file_name,
 (coalesce(asset.metadata_json->>'sensitive','false')='true' OR EXISTS(
 SELECT 1 FROM evidence_links evidence WHERE evidence.project_id=asset.project_id AND evidence.asset_id=asset.id AND evidence.is_published))::boolean AS sensitive
FROM assets asset LEFT JOIN asset_upload_intents intent ON intent.project_id=asset.project_id AND intent.asset_id=asset.id
WHERE asset.project_id=$1 AND asset.id=$2 AND asset.status='available';
