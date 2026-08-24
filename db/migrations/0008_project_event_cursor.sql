create table project_events (
  cursor bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  event_id text not null,
  event_type text not null,
  payload_json jsonb not null default '{}'::jsonb,
  occurred_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  constraint project_events_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint project_events_event_unique unique (event_id)
);
--> statement-breakpoint
create index project_events_project_cursor_idx on project_events(project_id, cursor);
--> statement-breakpoint
create index project_events_project_occurred_idx on project_events(project_id, occurred_at, cursor);
--> statement-breakpoint
create or replace function notify_aerosight_outbox() returns trigger as $$
begin
  perform pg_notify('aerosight_outbox', json_build_object(
    'projectId', new.project_id,
    'eventId', new.event_id
  )::text);
  return new;
end;
$$ language plpgsql;
--> statement-breakpoint
create trigger outbox_events_notify
after insert on outbox_events
for each row execute function notify_aerosight_outbox();
--> statement-breakpoint
create or replace function notify_aerosight_project_event() returns trigger as $$
begin
  perform pg_notify('aerosight_project_events', json_build_object(
    'projectId', new.project_id,
    'cursor', new.cursor
  )::text);
  return new;
end;
$$ language plpgsql;
--> statement-breakpoint
create trigger project_events_notify
after insert on project_events
for each row execute function notify_aerosight_project_event();
