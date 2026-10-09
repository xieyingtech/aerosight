-- name: ReadMediaAccessAsset :one
SELECT asset.id,asset.project_id,asset.storage_key,asset.mime_type,asset.kind,
 coalesce(nullif(asset.metadata_json->>'uploadFileName',''),nullif(asset.metadata_json->>'name',''),nullif(asset.metadata_json->>'fileName',''),nullif(asset.metadata_json->>'filename',''),'')::text AS file_name,
 (coalesce(asset.metadata_json->>'sensitive','false')='true' OR coalesce(asset.metadata_json->>'legacyPublishedEvidence','false')='true')::boolean AS sensitive
FROM assets asset
WHERE asset.project_id=$1 AND asset.id=$2 AND asset.status='available';
