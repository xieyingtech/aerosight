alter table device_connections
  add column heartbeat_interval_seconds integer not null default 30;
--> statement-breakpoint
alter table device_connections
  add column status_projected_at timestamptz;
--> statement-breakpoint
alter table device_connections
  add constraint device_connections_heartbeat_interval_valid
  check (heartbeat_interval_seconds between 5 and 3600);
--> statement-breakpoint
alter table devices
  add constraint devices_connectivity_status_valid
  check (status in ('online', 'degraded', 'offline', 'unknown'));
--> statement-breakpoint
create index device_connections_open_heartbeat_idx
  on device_connections(last_heartbeat_at) where closed_at is null;
