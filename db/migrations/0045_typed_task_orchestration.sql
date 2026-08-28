alter table task_versions
  add column input_schema_json jsonb not null default '{"type":"object","properties":{},"additionalProperties":false}'::jsonb,
  add column trigger_json jsonb not null default '{"type":"manual"}'::jsonb,
  add column concurrency_limit integer not null default 1,
  add constraint task_versions_input_schema_object check (jsonb_typeof(input_schema_json) = 'object'),
  add constraint task_versions_trigger_object check (jsonb_typeof(trigger_json) = 'object'),
  add constraint task_versions_concurrency_positive check (concurrency_limit > 0 and concurrency_limit <= 100);
--> statement-breakpoint
alter table task_versions disable trigger task_versions_published_immutable;
--> statement-breakpoint
update task_versions version
   set trigger_json = case
     when jsonb_typeof(version.definition_json->'trigger') = 'object' then version.definition_json->'trigger'
     when version.definition_json->>'triggerType' = 'schedule' then
       jsonb_build_object('type','schedule','cron',coalesce(version.definition_json->>'schedule','0 0 * * *'),'timezone','UTC')
     when version.definition_json->>'triggerType' = 'event' then
       jsonb_build_object('type','webhook','source','legacy-event')
     else '{"type":"manual"}'::jsonb
   end;
--> statement-breakpoint
alter table task_versions enable trigger task_versions_published_immutable;
--> statement-breakpoint
alter table task_steps
  add column uses text not null default 'device.command',
  add column input_schema_json jsonb not null default '{"type":"object","properties":{}}'::jsonb,
  add column output_schema_json jsonb not null default '{"type":"object","properties":{}}'::jsonb,
  add column condition_json jsonb,
  add column depends_on_json jsonb not null default '[]'::jsonb,
  add column timeout_seconds integer not null default 300,
  add column retry_policy_json jsonb not null default '{"maxAttempts":1,"backoffSeconds":0}'::jsonb,
  add constraint task_steps_uses_valid check (uses in ('device.command','device.collect','algorithm.run','issue.create-or-update','copilot.run','report.generate')),
  add constraint task_steps_input_schema_object check (jsonb_typeof(input_schema_json) = 'object'),
  add constraint task_steps_output_schema_object check (jsonb_typeof(output_schema_json) = 'object'),
  add constraint task_steps_condition_object check (condition_json is null or jsonb_typeof(condition_json) = 'object'),
  add constraint task_steps_depends_array check (jsonb_typeof(depends_on_json) = 'array'),
  add constraint task_steps_timeout_positive check (timeout_seconds > 0 and timeout_seconds <= 86400),
  add constraint task_steps_retry_policy_object check (jsonb_typeof(retry_policy_json) = 'object');
--> statement-breakpoint
alter table task_steps disable trigger task_steps_published_immutable;
--> statement-breakpoint
update task_steps step
   set uses = case
     when coalesce((step.media_requirements_json->>'required')::boolean,false) then 'device.collect'
     else 'device.command'
   end,
   depends_on_json = coalesce((
     select jsonb_build_array(previous.step_key)
       from task_steps previous
      where previous.task_version_id=step.task_version_id and previous.position < step.position
      order by previous.position desc limit 1
   ),'[]'::jsonb),
   retry_policy_json = jsonb_build_object(
     'maxAttempts', greatest(1,coalesce((step.failure_policy_json->>'maxRetries')::integer,0)+1),
     'backoffSeconds', greatest(0,coalesce((step.failure_policy_json->>'retryBackoffSeconds')::integer,0))
   );
--> statement-breakpoint
alter table task_steps enable trigger task_steps_published_immutable;
