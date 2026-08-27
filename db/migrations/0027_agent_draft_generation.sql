alter table agent_drafts add column model_id text;
alter table agent_drafts add column prompt_template_version text;
alter table agent_drafts add column generation_tool_calls_json jsonb not null default '[]'::jsonb;
alter table agent_drafts add column evidence_version_hash text;
alter table agent_drafts add column generated_at timestamptz;

alter table agent_drafts add constraint agent_drafts_generation_metadata_complete check(
  (model_id is null and prompt_template_version is null and evidence_version_hash is null and generated_at is null)
  or (model_id is not null and prompt_template_version is not null and evidence_version_hash is not null and generated_at is not null)
);
