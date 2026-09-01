create table connector_asset_access_refs (
  id integer primary key,
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  remote_resource_id bigint not null,
  access_kind text not null,
  reference_digest text not null,
  credential_envelope_json jsonb not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_asset_access_refs_project_team_fk
    foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_asset_access_refs_connector_project_fk
    foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_asset_access_refs_resource_project_fk
    foreign key(remote_resource_id,project_id) references connector_remote_resources(id,project_id) on delete cascade,
  constraint connector_asset_access_refs_asset_project_fk
    foreign key(id,project_id) references assets(id,project_id) on delete cascade,
  constraint connector_asset_access_refs_kind_valid
    check(access_kind in('flight-media','flight-record')),
  constraint connector_asset_access_refs_digest_valid
    check(reference_digest ~ '^[a-f0-9]{64}$'),
  constraint connector_asset_access_refs_envelope_object
    check(jsonb_typeof(credential_envelope_json)='object'),
  constraint connector_asset_access_refs_id_project_unique unique(id,project_id),
  constraint connector_asset_access_refs_reference_unique
    unique(project_id,connector_instance_id,access_kind,reference_digest)
);
--> statement-breakpoint
create index connector_asset_access_refs_resource_idx
  on connector_asset_access_refs(project_id,connector_instance_id,remote_resource_id);
