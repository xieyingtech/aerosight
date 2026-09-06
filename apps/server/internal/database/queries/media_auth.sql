-- name: ReadMediaPublishCredential :one
SELECT stream.project_id,adapter.id AS adapter_id,adapter.credential_envelope_json
FROM live_streams stream JOIN device_adapters adapter ON adapter.id=stream.adapter_id AND adapter.project_id=stream.project_id
WHERE stream.ingest_ref=$1 AND stream.status IN ('requested','starting','live','degraded') LIMIT 1;
