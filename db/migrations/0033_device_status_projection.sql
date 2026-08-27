alter table devices
  add column status_observed_at timestamptz,
  add column status_projected_at timestamptz not null default now(),
  add column data_freshness text not null default 'unknown',
  add column raw_status_ref text;

update devices
set status_observed_at = last_seen_at,
    data_freshness = case
      when last_seen_at is null then 'unknown'
      when status = 'offline' then 'expired'
      else 'stale'
    end;

alter table devices add constraint devices_data_freshness_valid
  check (data_freshness in ('fresh', 'stale', 'expired', 'unknown'));
