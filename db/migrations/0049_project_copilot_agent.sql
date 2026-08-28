create unique index agents_project_copilot_unique
  on agents(project_id,(config_json->>'kind'))
  where config_json->>'kind'='copilot';
--> statement-breakpoint
create or replace function provision_project_copilot_agent() returns trigger language plpgsql as $$
begin
  insert into agents(project_id,name,description,status,config_json)
  values(new.id,'Copilot','项目级 AI 助手，可通过案件评论提及或负责人指派触发。','active',
         '{"kind":"copilot","builtIn":true}'::jsonb);
  return new;
end $$;
--> statement-breakpoint
create trigger projects_provision_copilot_agent
  after insert on projects for each row execute function provision_project_copilot_agent();
--> statement-breakpoint
insert into agents(project_id,name,description,status,config_json)
select project.id,'Copilot','项目级 AI 助手，可通过案件评论提及或负责人指派触发。','active',
       '{"kind":"copilot","builtIn":true}'::jsonb
  from projects project
 where not exists (
   select 1 from agents agent
    where agent.project_id=project.id and agent.config_json->>'kind'='copilot'
 );
