create table device_protocol_messages (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  adapter_id bigint not null,
  gateway_sn text not null,
  device_sn text not null,
  topic text not null,
  route_kind text not null,
  transaction_id text not null,
  business_id text,
  method text,
  timestamp_ms bigint not null,
  sequence_number bigint,
  qos smallint not null,
  duplicate_flag boolean not null default false,
  payload_json jsonb not null,
  disposition text not null default 'accepted',
  disposition_reason text,
  received_at timestamptz not null default now(),
  constraint device_protocol_messages_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_protocol_messages_adapter_project_fk
    foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint device_protocol_messages_route_valid
    check (route_kind in ('topology', 'state', 'telemetry', 'event', 'request', 'service_reply')),
  constraint device_protocol_messages_disposition_valid
    check (disposition in ('accepted', 'out_of_order')),
  constraint device_protocol_messages_adapter_topic_tid_unique
    unique (adapter_id, topic, transaction_id)
);
--> statement-breakpoint
create index device_protocol_messages_project_time_idx
  on device_protocol_messages(project_id, received_at desc);
--> statement-breakpoint
create table device_protocol_cursors (
  project_id integer not null,
  team_id integer not null,
  adapter_id bigint not null,
  route_key text not null,
  last_timestamp_ms bigint not null,
  last_transaction_id text not null,
  updated_at timestamptz not null default now(),
  primary key (adapter_id, route_key),
  constraint device_protocol_cursors_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_protocol_cursors_adapter_project_fk
    foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete cascade
);
