alter table devices
  add column uav_registration_number text,
  add column registration_valid_until timestamptz,
  add column remote_identification_code text,
  add column responsible_user_id integer,
  add constraint devices_responsible_user_fk
    foreign key (responsible_user_id) references users(id) on delete set null;
--> statement-breakpoint
alter table task_runs
  add column operation_approval_reference text,
  add column operation_approval_valid_until timestamptz,
  add column takeoff_confirmed_at timestamptz,
  add column takeoff_confirmed_by_user_id integer,
  add column responsible_user_id integer,
  add column incident_report_reference text,
  add column incident_reported_at timestamptz,
  add constraint task_runs_takeoff_confirmer_fk
    foreign key (takeoff_confirmed_by_user_id) references users(id) on delete restrict,
  add constraint task_runs_responsible_user_fk
    foreign key (responsible_user_id) references users(id) on delete set null,
  add constraint task_runs_takeoff_confirmation_complete
    check ((takeoff_confirmed_at is null) = (takeoff_confirmed_by_user_id is null)),
  add constraint task_runs_incident_report_complete
    check ((incident_report_reference is null) = (incident_reported_at is null));
--> statement-breakpoint
create index devices_registration_number_idx
  on devices(uav_registration_number) where uav_registration_number is not null;
--> statement-breakpoint
create index task_runs_responsible_created_idx
  on task_runs(responsible_user_id, created_at desc) where responsible_user_id is not null;
