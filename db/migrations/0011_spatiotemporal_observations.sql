create table coordinate_references (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  code text not null,
  name text not null,
  authority text,
  definition text,
  vertical_datum text,
  transform_version text not null default '1',
  is_project_standard boolean not null default false,
  created_at timestamptz not null default now(),
  constraint coordinate_references_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint coordinate_references_project_code_version_unique
    unique (project_id, code, transform_version),
  constraint coordinate_references_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create unique index coordinate_references_one_standard_idx
  on coordinate_references(project_id) where is_project_standard;
--> statement-breakpoint
create table sensor_calibrations (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  device_id integer not null,
  sensor_key text not null,
  version integer not null,
  intrinsic_json jsonb not null default '{}'::jsonb,
  extrinsic_json jsonb not null default '{}'::jsonb,
  quality_json jsonb not null default '{}'::jsonb,
  valid_from timestamptz not null,
  valid_until timestamptz,
  created_at timestamptz not null default now(),
  constraint sensor_calibrations_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint sensor_calibrations_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint sensor_calibrations_device_sensor_version_unique
    unique (device_id, sensor_key, version),
  constraint sensor_calibrations_id_project_unique unique (id, project_id),
  constraint sensor_calibrations_valid_range check (valid_until is null or valid_until > valid_from)
);
--> statement-breakpoint
create index sensor_calibrations_device_valid_idx
  on sensor_calibrations(device_id, sensor_key, valid_from desc);
--> statement-breakpoint
create table observations (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  adapter_id bigint not null,
  device_id integer not null,
  calibration_id bigint,
  observation_type text not null,
  source_event_id text not null,
  captured_at timestamptz not null,
  received_at timestamptz not null,
  time_quality text not null default 'trusted',
  original_crs_id bigint,
  original_geometry geometry(GeometryZ),
  standard_geometry geometry(GeometryZ, 4326),
  properties_json jsonb not null default '{}'::jsonb,
  quality_json jsonb not null default '{}'::jsonb,
  validity text not null default 'valid',
  created_at timestamptz not null default now(),
  constraint observations_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint observations_adapter_project_fk
    foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint observations_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint observations_calibration_project_fk
    foreign key (calibration_id, project_id) references sensor_calibrations(id, project_id) on delete set null (calibration_id),
  constraint observations_crs_project_fk
    foreign key (original_crs_id, project_id) references coordinate_references(id, project_id) on delete set null (original_crs_id),
  constraint observations_source_unique unique (adapter_id, source_event_id),
  constraint observations_id_project_unique unique (id, project_id),
  constraint observations_time_quality_valid check (time_quality in ('trusted', 'uncertain', 'invalid')),
  constraint observations_validity_valid check (validity in ('valid', 'degraded', 'late', 'invalid'))
);
--> statement-breakpoint
create index observations_project_time_idx on observations(project_id, captured_at desc, id);
--> statement-breakpoint
create index observations_device_type_time_idx on observations(device_id, observation_type, captured_at desc);
--> statement-breakpoint
create index observations_standard_geometry_gist on observations using gist(standard_geometry);
--> statement-breakpoint
create index observations_original_geometry_gist on observations using gist(original_geometry);
--> statement-breakpoint
create table poses (
  observation_id bigint primary key,
  project_id integer not null,
  device_id integer not null,
  captured_at timestamptz not null,
  standard_position geometry(PointZ, 4326),
  original_position geometry(PointZ),
  orientation_x double precision,
  orientation_y double precision,
  orientation_z double precision,
  orientation_w double precision,
  velocity_x double precision,
  velocity_y double precision,
  velocity_z double precision,
  horizontal_accuracy_m double precision,
  vertical_accuracy_m double precision,
  attitude_accuracy_deg double precision,
  vertical_datum text,
  transform_version text,
  spatial_quality text not null default 'usable',
  constraint poses_observation_project_fk
    foreign key (observation_id, project_id) references observations(id, project_id) on delete cascade,
  constraint poses_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint poses_spatial_quality_valid check (spatial_quality in ('usable', 'degraded', 'unusable')),
  constraint poses_accuracy_nonnegative check (
    (horizontal_accuracy_m is null or horizontal_accuracy_m >= 0) and
    (vertical_accuracy_m is null or vertical_accuracy_m >= 0) and
    (attitude_accuracy_deg is null or attitude_accuracy_deg >= 0)
  )
);
--> statement-breakpoint
create index poses_project_time_idx on poses(project_id, captured_at desc, observation_id);
--> statement-breakpoint
create index poses_device_time_idx on poses(device_id, captured_at desc, observation_id);
--> statement-breakpoint
create index poses_standard_position_gist on poses using gist(standard_position);
