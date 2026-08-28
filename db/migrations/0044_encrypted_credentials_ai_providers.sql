alter table device_adapters
  add column credential_envelope_json jsonb,
  add constraint device_adapters_credential_envelope_object
    check (credential_envelope_json is null or jsonb_typeof(credential_envelope_json) = 'object');
--> statement-breakpoint
alter table algorithm_providers
  add column credential_envelope_json jsonb,
  add constraint algorithm_providers_credential_envelope_object
    check (credential_envelope_json is null or jsonb_typeof(credential_envelope_json) = 'object');
--> statement-breakpoint
create table ai_providers (
  id bigserial primary key,
  name text not null,
  provider_type text not null,
  base_url text,
  model_id text not null,
  credential_envelope_json jsonb not null,
  enabled boolean not null default false,
  is_default boolean not null default false,
  status text not null default 'untested',
  health_json jsonb not null default '{}'::jsonb,
  last_tested_at timestamptz,
  created_by_user_id integer not null references users(id) on delete restrict,
  updated_by_user_id integer not null references users(id) on delete restrict,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint ai_providers_name_unique unique (name),
  constraint ai_providers_type_valid check (provider_type in ('openai')),
  constraint ai_providers_status_valid check (status in ('untested','healthy','degraded','failed')),
  constraint ai_providers_default_enabled check (not is_default or enabled),
  constraint ai_providers_credential_envelope_object check (jsonb_typeof(credential_envelope_json) = 'object')
);
--> statement-breakpoint
create unique index ai_providers_single_default_idx on ai_providers(is_default) where is_default;
--> statement-breakpoint
create table platform_audit_events (
  id bigserial primary key,
  actor_user_id integer not null references users(id) on delete restrict,
  request_id text not null,
  action text not null,
  resource_type text not null,
  resource_id text,
  input_hash text not null,
  result_hash text,
  status text not null default 'accepted',
  created_at timestamptz not null default now(),
  completed_at timestamptz,
  constraint platform_audit_events_status_valid check (status in ('accepted','completed'))
);
--> statement-breakpoint
create index platform_audit_events_actor_created_idx on platform_audit_events(actor_user_id, created_at desc);
--> statement-breakpoint
update device_adapters
   set status = 'disabled',
       last_health_json = jsonb_build_object('ok', false, 'code', 'CREDENTIAL_REENTRY_REQUIRED'),
       updated_at = now()
 where secret_ref is not null and credential_envelope_json is null;
--> statement-breakpoint
update algorithm_providers
   set status = 'disabled',
       health_json = jsonb_build_object('ok', false, 'code', 'CREDENTIAL_REENTRY_REQUIRED'),
       updated_at = now()
 where secret_ref is not null and credential_envelope_json is null;
