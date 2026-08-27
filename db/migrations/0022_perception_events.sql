create table event_rules (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  name text not null,
  status text not null default 'disabled',
  current_published_version_id bigint,
  created_by_user_id integer,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint event_rules_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint event_rules_creator_fk foreign key(created_by_user_id) references users(id) on delete set null,
  constraint event_rules_status_valid check(status in ('disabled','active','retired')),
  constraint event_rules_project_name_unique unique(project_id,name),
  constraint event_rules_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create table event_rule_versions (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  event_rule_id bigint not null,
  version integer not null,
  status text not null default 'draft',
  label text not null,
  minimum_confidence double precision not null,
  severity text not null,
  deduplication_window_seconds integer not null default 3600,
  conditions_json jsonb not null default '{}'::jsonb,
  created_by_user_id integer,
  published_by_user_id integer,
  created_at timestamptz not null default now(),
  published_at timestamptz,
  constraint event_rule_versions_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint event_rule_versions_rule_project_fk foreign key(event_rule_id,project_id) references event_rules(id,project_id) on delete cascade,
  constraint event_rule_versions_creator_fk foreign key(created_by_user_id) references users(id) on delete set null,
  constraint event_rule_versions_publisher_fk foreign key(published_by_user_id) references users(id) on delete set null,
  constraint event_rule_versions_status_valid check(status in ('draft','published','retired')),
  constraint event_rule_versions_confidence_valid check(minimum_confidence between 0 and 1),
  constraint event_rule_versions_severity_valid check(severity in ('low','medium','high','critical')),
  constraint event_rule_versions_window_valid check(deduplication_window_seconds > 0),
  constraint event_rule_versions_rule_version_unique unique(event_rule_id,version),
  constraint event_rule_versions_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
alter table event_rules add constraint event_rules_current_version_project_fk foreign key(current_published_version_id,project_id) references event_rule_versions(id,project_id) on delete set null(current_published_version_id);
--> statement-breakpoint
create unique index event_rule_versions_one_draft_idx on event_rule_versions(event_rule_id) where status='draft';
--> statement-breakpoint
create table perception_events (
  id uuid primary key,
  project_id integer not null,
  team_id integer not null,
  event_rule_version_id bigint not null,
  detection_group_id bigint not null,
  deduplication_key text not null,
  title text not null default '疑似违建',
  severity text not null,
  status text not null default 'open',
  occurrence_count integer not null default 1,
  state_version integer not null default 0,
  assigned_user_id integer,
  first_detected_at timestamptz not null,
  last_detected_at timestamptz not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  resolved_at timestamptz,
  constraint perception_events_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint perception_events_rule_version_project_fk foreign key(event_rule_version_id,project_id) references event_rule_versions(id,project_id) on delete restrict,
  constraint perception_events_group_project_fk foreign key(detection_group_id,project_id) references detection_groups(id,project_id) on delete restrict,
  constraint perception_events_assignee_fk foreign key(assigned_user_id) references users(id) on delete set null,
  constraint perception_events_severity_valid check(severity in ('low','medium','high','critical')),
  constraint perception_events_status_valid check(status in ('open','acknowledged','investigating','resolved','dismissed')),
  constraint perception_events_counts_valid check(occurrence_count > 0 and state_version >= 0 and last_detected_at >= first_detected_at),
  constraint perception_events_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create unique index perception_events_active_dedup_idx on perception_events(project_id,deduplication_key) where status in ('open','acknowledged','investigating');
--> statement-breakpoint
create table event_feedback (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  perception_event_id uuid not null,
  action text not null,
  value_json jsonb not null default '{}'::jsonb,
  reason text not null,
  actor_user_id integer not null,
  created_at timestamptz not null default now(),
  constraint event_feedback_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint event_feedback_event_project_fk foreign key(perception_event_id,project_id) references perception_events(id,project_id) on delete cascade,
  constraint event_feedback_actor_fk foreign key(actor_user_id) references users(id) on delete restrict,
  constraint event_feedback_action_valid check(action in ('confirm','false_positive','category_correction','assign','acknowledge','investigate','dismiss','resolve'))
);
--> statement-breakpoint
create or replace function protect_published_event_rule_version() returns trigger language plpgsql as $$ begin
  if old.status in ('published','retired') then raise exception 'published event rule versions are immutable' using errcode='55000'; end if;
  return case when tg_op='DELETE' then old else new end;
end; $$;
--> statement-breakpoint
create trigger event_rule_versions_published_immutable before update or delete on event_rule_versions for each row execute function protect_published_event_rule_version();
--> statement-breakpoint
create index perception_events_project_status_idx on perception_events(project_id,status,last_detected_at desc);
--> statement-breakpoint
create index event_feedback_event_created_idx on event_feedback(perception_event_id,created_at);
