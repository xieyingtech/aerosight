-- The governed action owns a separate physical flight Run. Business workflow
-- completion must never be driven by the action's projection state updates.
alter table connector_action_jobs add constraint connector_action_jobs_inspection_scope_unique
 unique(id,project_id,team_id,connector_instance_id,task_run_id,action_kind);

create table inspection_flight_bindings (
 project_id integer not null,
 team_id integer not null,
 business_run_id integer not null,
 business_step_id bigint not null,
 connector_instance_id bigint not null,
 flight_run_id integer not null,
 action_job_id uuid not null,
 action_kind text not null default 'flight-task-create' check(action_kind='flight-task-create'),
 created_at timestamptz not null default now(),
 primary key(project_id,business_step_id),
 unique(action_job_id),
 unique(project_id,flight_run_id),
 check(business_run_id<>flight_run_id),
 foreign key(project_id,team_id) references projects(id,team_id),
 foreign key(business_step_id,business_run_id,project_id) references task_run_steps(id,task_run_id,project_id),
 foreign key(action_job_id,project_id,team_id,connector_instance_id,flight_run_id,action_kind)
 references connector_action_jobs(id,project_id,team_id,connector_instance_id,task_run_id,action_kind)
);
