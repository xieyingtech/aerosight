alter table live_streams add column vendor_stream_ref text;
--> statement-breakpoint
alter table device_commands add column live_stream_id bigint;
--> statement-breakpoint
alter table device_commands add constraint device_commands_live_stream_project_fk
  foreign key (live_stream_id, project_id)
  references live_streams(id, project_id) on delete set null (live_stream_id);
--> statement-breakpoint
create unique index device_commands_live_stream_action_unique
  on device_commands(live_stream_id, command_key) where live_stream_id is not null;
