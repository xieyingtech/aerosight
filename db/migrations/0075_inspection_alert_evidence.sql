-- Private source identity is separated from public connector summaries. This
-- also retains evidence while Task ownership is pending, without creating cases.
alter table connector_remote_resources add constraint connector_remote_resources_connector_project_unique unique(id,project_id,connector_instance_id);

create table inspection_alert_sources (
  project_id integer not null,
  connector_instance_id bigint not null,
  remote_resource_id bigint not null,
  remote_flight_id text not null,
  evidence_json jsonb not null,
  updated_at timestamptz not null default now(),
  primary key(project_id,remote_resource_id),
  foreign key(project_id) references projects(id) on delete cascade,
  foreign key(connector_instance_id,project_id) references device_adapters(id,project_id),
  foreign key(remote_resource_id,project_id,connector_instance_id) references connector_remote_resources(id,project_id,connector_instance_id),
  check(length(remote_flight_id)>0)
);
create index inspection_alert_sources_flight_idx on inspection_alert_sources(project_id,connector_instance_id,remote_flight_id);

alter table inspection_flight_ownership add constraint inspection_flight_ownership_project_fk foreign key(project_id) references projects(id) on delete cascade;
