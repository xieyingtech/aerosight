alter table device_adapters
  add column lease_owner text,
  add column lease_expires_at timestamptz,
  add column connection_epoch bigint not null default 0,
  add column last_connected_at timestamptz;
--> statement-breakpoint
alter table device_adapters add constraint device_adapters_lease_complete check (
  (lease_owner is null and lease_expires_at is null)
  or (lease_owner is not null and lease_expires_at is not null)
);
--> statement-breakpoint
create index device_adapters_lease_claim_idx
  on device_adapters(adapter_type, status, lease_expires_at)
  where status in ('connecting', 'connected', 'degraded');
