create table detections (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  algorithm_run_id uuid not null,
  input_asset_id integer not null,
  task_run_id integer,
  detection_key text not null,
  label text not null,
  confidence double precision not null,
  pixel_geometry_json jsonb not null,
  geographic_geometry geometry(Polygon,4326),
  location_quality text not null default 'unavailable',
  projection_method text not null default 'image-only',
  horizontal_error_meters double precision,
  transform_version text not null,
  attributes_json jsonb not null default '{}'::jsonb,
  captured_at timestamptz not null,
  created_at timestamptz not null default now(),
  constraint detections_project_team_fk foreign key (project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint detections_run_project_fk foreign key (algorithm_run_id,project_id) references algorithm_runs(id,project_id) on delete cascade,
  constraint detections_asset_project_fk foreign key (input_asset_id,project_id) references assets(id,project_id) on delete restrict,
  constraint detections_task_run_project_fk foreign key (task_run_id,project_id) references task_runs(id,project_id) on delete set null (task_run_id),
  constraint detections_confidence_valid check (confidence between 0 and 1),
  constraint detections_location_quality_valid check (location_quality in ('surveyed','estimated','low','unavailable')),
  constraint detections_error_valid check (horizontal_error_meters is null or horizontal_error_meters >= 0),
  constraint detections_run_key_unique unique (algorithm_run_id,detection_key),
  constraint detections_id_project_unique unique (id,project_id)
);
--> statement-breakpoint
create table detection_groups (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  label text not null,
  status text not null default 'active',
  geographic_geometry geometry(Polygon,4326),
  location_quality text not null,
  first_detected_at timestamptz not null,
  last_detected_at timestamptz not null,
  member_count integer not null default 1,
  aggregation_version text not null default 'aerosight-detection-aggregation/v1',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint detection_groups_project_team_fk foreign key (project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint detection_groups_status_valid check (status in ('active','superseded')),
  constraint detection_groups_location_quality_valid check (location_quality in ('surveyed','estimated','low','unavailable')),
  constraint detection_groups_time_valid check (last_detected_at >= first_detected_at and member_count > 0),
  constraint detection_groups_id_project_unique unique (id,project_id)
);
--> statement-breakpoint
create table detection_group_members (
  project_id integer not null,
  team_id integer not null,
  detection_group_id bigint not null,
  detection_id bigint not null,
  added_at timestamptz not null default now(),
  primary key (detection_group_id,detection_id),
  constraint detection_group_members_project_team_fk foreign key (project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint detection_group_members_group_project_fk foreign key (detection_group_id,project_id) references detection_groups(id,project_id) on delete cascade,
  constraint detection_group_members_detection_project_fk foreign key (detection_id,project_id) references detections(id,project_id) on delete cascade,
  constraint detection_group_members_detection_unique unique (detection_id)
);
--> statement-breakpoint
create index detections_project_captured_idx on detections(project_id,captured_at desc);
--> statement-breakpoint
create index detections_geometry_gist on detections using gist(geographic_geometry);
--> statement-breakpoint
create index detection_groups_project_time_idx on detection_groups(project_id,last_detected_at desc);
--> statement-breakpoint
create index detection_groups_geometry_gist on detection_groups using gist(geographic_geometry);
