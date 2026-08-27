alter table live_streams drop constraint live_streams_status_valid;
--> statement-breakpoint
alter table live_streams add constraint live_streams_status_valid
  check (status in ('requested', 'starting', 'live', 'degraded', 'failed', 'stopping', 'stopped'));
--> statement-breakpoint
alter table live_streams
  add column session_token uuid not null default gen_random_uuid(),
  add column lease_owner text;
--> statement-breakpoint
update live_streams set lease_owner='legacy' where lease_expires_at is not null and lease_owner is null;
--> statement-breakpoint
alter table live_streams add constraint live_streams_lease_complete check (
  (lease_owner is null and lease_expires_at is null)
  or (lease_owner is not null and lease_expires_at is not null)
);
--> statement-breakpoint
drop index live_streams_one_active_device_key_idx;
--> statement-breakpoint
create unique index live_streams_one_active_device_key_idx
  on live_streams(project_id, device_id, stream_key)
  where status in ('requested', 'starting', 'live', 'degraded', 'stopping');
--> statement-breakpoint
create unique index live_streams_ingest_ref_unique
  on live_streams(ingest_ref) where ingest_ref is not null;
--> statement-breakpoint
create index live_streams_expired_lease_idx
  on live_streams(lease_expires_at)
  where status in ('requested', 'starting', 'live', 'degraded', 'stopping');
