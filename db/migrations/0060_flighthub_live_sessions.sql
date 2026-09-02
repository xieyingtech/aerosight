alter table live_streams
  add column supplier text,
  add column supplier_protocol text,
  add column supplier_adapter_version text,
  add column supplier_reference_digest text,
  add column supplier_credential_expires_at timestamptz,
  add column supplier_credential_envelope_json jsonb,
  add column start_attempted_at timestamptz,
  add column start_accepted_at timestamptz,
  add column last_playback_at timestamptz,
  add column local_authorization_revoked_at timestamptz,
  add column remote_evidence_at timestamptz,
  add constraint live_streams_supplier_digest_valid check (
    supplier_reference_digest is null or supplier_reference_digest ~ '^[a-f0-9]{64}$'
  ),
  add constraint live_streams_supplier_envelope_object check (
    supplier_credential_envelope_json is null
    or jsonb_typeof(supplier_credential_envelope_json) = 'object'
  ),
  add constraint live_streams_supplier_credential_complete check (
    supplier_credential_envelope_json is null
    or (
      source_type = 'dji_flighthub'
      and supplier is not null
      and supplier_protocol is not null
      and supplier_adapter_version is not null
      and supplier_reference_digest is not null
      and supplier_credential_expires_at is not null
      and start_accepted_at is not null
      and local_authorization_revoked_at is null
    )
  ),
  add constraint live_streams_flighthub_adapter_required check (
    source_type <> 'dji_flighthub' or adapter_id is not null
  );
--> statement-breakpoint
create index live_streams_flighthub_reconcile_idx
  on live_streams(adapter_id, status, updated_at)
  where source_type = 'dji_flighthub'
    and status in ('requested','starting','live','degraded','stopping');
