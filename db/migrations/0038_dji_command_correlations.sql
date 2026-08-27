alter table device_commands alter column task_run_id drop not null;
--> statement-breakpoint
create table device_command_protocol_correlations (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  command_id uuid not null,
  adapter_id bigint not null,
  mapping_version text not null,
  transaction_id text not null,
  business_id text not null,
  method text not null,
  request_topic text not null,
  request_payload_json jsonb not null,
  status text not null default 'prepared',
  reply_event_id text,
  reply_result integer,
  reply_payload_json jsonb,
  sent_at timestamptz,
  replied_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint device_command_protocol_correlations_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_command_protocol_correlations_command_project_fk
    foreign key (command_id, project_id) references device_commands(id, project_id) on delete cascade,
  constraint device_command_protocol_correlations_adapter_project_fk
    foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint device_command_protocol_correlations_status_valid
    check (status in ('prepared','sent','acknowledged','nacked')),
  constraint device_command_protocol_correlations_command_unique unique (command_id),
  constraint device_command_protocol_correlations_transaction_unique unique (adapter_id, transaction_id),
  constraint device_command_protocol_correlations_business_method_unique unique (adapter_id, business_id, method)
);
--> statement-breakpoint
create index device_command_protocol_correlations_reply_idx
  on device_command_protocol_correlations(adapter_id, transaction_id, business_id, method, status);
