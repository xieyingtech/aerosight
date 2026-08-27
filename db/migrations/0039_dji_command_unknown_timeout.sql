alter table device_command_protocol_correlations
  drop constraint device_command_protocol_correlations_status_valid;
--> statement-breakpoint
alter table device_command_protocol_correlations
  add constraint device_command_protocol_correlations_status_valid
  check (status in ('prepared','sent','acknowledged','nacked','unknown'));
