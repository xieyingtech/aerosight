\set ON_ERROR_STOP on

begin;
select pg_advisory_xact_lock(hashtext('aerosight.dji.local-demo'));

do $$
declare
  admin_id integer;
  demo_team_id integer;
begin
  select id into admin_id from users where email = 'admin@example.com';
  if admin_id is null then
    raise exception 'default admin has not been bootstrapped by the web application';
  end if;

  select team.id into demo_team_id
    from teams team
    join projects project on project.team_id = team.id
   where project.name = 'DJI Local Demo'
   order by project.id limit 1;
  if demo_team_id is null then
    insert into teams(name) values ('AeroSight Local Demo') returning id into demo_team_id;
  end if;

  insert into team_members(team_id, user_id, role)
  select demo_team_id, admin_id, 'owner'
  where not exists (
    select 1 from team_members where team_id = demo_team_id and user_id = admin_id
  );

  insert into projects(team_id, name, description, created_by_user_id)
  select demo_team_id, 'DJI Local Demo', 'Protocol-level Dock 2 and Dock 3 demonstration', admin_id
  where not exists (
    select 1 from projects where team_id = demo_team_id and name = 'DJI Local Demo'
  );
end $$;

with demo as (
  select project.id as project_id, project.team_id
    from projects project where project.name = 'DJI Local Demo'
   order by project.id limit 1
)
insert into device_network_profiles (
  project_id, team_id, name, mode, mqtt_endpoint, api_public_base_url,
  websocket_public_url, media_ingest_base_url, media_playback_base_url,
  tls_required, secret_ref, status, config_json, last_validation_json, last_validated_at
)
select project_id, team_id, 'DJI Local LAN', 'lan', :'mqtt_endpoint', :'web_base_url',
       :'websocket_url', :'media_ingest_url', :'media_playback_url', false,
       'env://DJI_DEMO_MQTT_CREDENTIALS', 'valid',
       jsonb_build_object('mqttAnonymous', false, 'webrtcPlaybackBaseUrl', :'webrtc_playback_url'),
       '{"status":"valid","code":"LOCAL_DEMO_BOOTSTRAP"}'::jsonb, now()
  from demo
on conflict (project_id, name) do update set
  mqtt_endpoint = excluded.mqtt_endpoint,
  api_public_base_url = excluded.api_public_base_url,
  websocket_public_url = excluded.websocket_public_url,
  media_ingest_base_url = excluded.media_ingest_base_url,
  media_playback_base_url = excluded.media_playback_base_url,
  secret_ref = excluded.secret_ref,
  status = 'valid', config_json = excluded.config_json,
  last_validation_json = excluded.last_validation_json, last_validated_at = now(), updated_at = now();

with demo as (
  select project.id as project_id, project.team_id, profile.id as profile_id
    from projects project
    join device_network_profiles profile on profile.project_id = project.id and profile.name = 'DJI Local LAN'
   where project.name = 'DJI Local Demo'
   order by project.id limit 1
), adapters(name, gateway_sn, aircraft_sn) as (
  values
    ('DJI Dock 2 Demo', 'DOCK2-DEMO-001', 'M3TD-DEMO-001'),
    ('DJI Dock 3 Demo', 'DOCK3-DEMO-001', 'M4TD-DEMO-001')
)
insert into device_adapters (
  project_id, team_id, name, adapter_type, vendor, protocol_version,
  status, secret_ref, config_json, network_profile_id
)
select demo.project_id, demo.team_id, adapters.name, 'dji', 'dji', 'cloud-api-mqtt5',
       'connecting', 'env://DJI_DEMO_MQTT_CREDENTIALS',
       jsonb_build_object(
         'clientId', 'aerosight-' || lower(replace(adapters.gateway_sn, '-', '')),
         'gatewaySerials', jsonb_build_array(adapters.gateway_sn),
         'topics', jsonb_build_array(
           'sys/product/' || adapters.gateway_sn || '/status',
           'thing/product/' || adapters.gateway_sn || '/state',
           'thing/product/' || adapters.gateway_sn || '/osd',
           'thing/product/' || adapters.gateway_sn || '/events',
           'thing/product/' || adapters.gateway_sn || '/requests',
           'thing/product/' || adapters.gateway_sn || '/services_reply',
           'thing/product/' || adapters.aircraft_sn || '/state',
           'thing/product/' || adapters.aircraft_sn || '/osd'
         )
       ), demo.profile_id
  from demo cross join adapters
on conflict (project_id, name) do update set
  status = 'connecting', secret_ref = excluded.secret_ref,
  config_json = excluded.config_json, network_profile_id = excluded.network_profile_id,
  lease_owner = null, lease_expires_at = null, updated_at = now();

with demo as (
  select project.id as project_id, project.team_id
    from projects project where project.name = 'DJI Local Demo'
   order by project.id limit 1
)
insert into assets (
  project_id, team_id, kind, mime_type, storage_key, size_bytes, checksum,
  checksum_sha256, logical_key, version, status, available_at, metadata_json
)
select project_id, team_id, 'document', 'application/json', :'algorithm_asset_key',
       :'algorithm_asset_size'::bigint, :'algorithm_asset_checksum', :'algorithm_asset_checksum',
       'demo/generic-ocr-input', 1, 'available', now(),
       '{"purpose":"generic-algorithm-demo"}'::jsonb
  from demo
on conflict (project_id, logical_key, version) do nothing;

with demo as (
  select project.id as project_id, project.team_id, member.user_id
    from projects project
    join team_members member on member.team_id = project.team_id and member.role = 'owner'
   where project.name = 'DJI Local Demo'
   order by project.id, member.user_id limit 1
)
insert into algorithm_providers (
  project_id, team_id, name, provider_type, base_url, auth_type, timeout_seconds,
  concurrency_limit, rate_limit_per_minute, status, health_json, created_by_user_id
)
select project_id, team_id, 'Local Generic Algorithm', 'http-json', :'algorithm_base_url',
       'none', 10, 2, 120, 'active', '{"status":"local_demo"}'::jsonb, user_id
  from demo
on conflict (project_id, name) do update set
  base_url = excluded.base_url, status = 'active', health_json = excluded.health_json,
  updated_at = now();

with demo as (
  select project.id as project_id, project.team_id, member.user_id,
         provider.id as provider_id
    from projects project
    join team_members member on member.team_id = project.team_id and member.role = 'owner'
    join algorithm_providers provider on provider.project_id = project.id
      and provider.name = 'Local Generic Algorithm'
   where project.name = 'DJI Local Demo'
   order by project.id, member.user_id limit 1
)
insert into algorithm_definitions (
  project_id, team_id, provider_id, name, capability_code, description, created_by_user_id
)
select project_id, team_id, provider_id, '通用文档 OCR', 'perception.ocr',
       '由动态 schema 定义的通用 OCR 演示，不绑定违建或其他业务类别。', user_id
  from demo
on conflict (project_id, name) do nothing;

with demo as (
  select definition.project_id, definition.team_id, definition.id as definition_id,
         member.user_id, asset.id as asset_id
    from algorithm_definitions definition
    join projects project on project.id = definition.project_id and project.name = 'DJI Local Demo'
    join team_members member on member.team_id = definition.team_id and member.role = 'owner'
    join assets asset on asset.project_id = definition.project_id
      and asset.logical_key = 'demo/generic-ocr-input' and asset.version = 1
   where definition.name = '通用文档 OCR'
   order by definition.id, member.user_id limit 1
)
insert into algorithm_definition_versions (
  project_id, team_id, algorithm_definition_id, version, status, execution_mode,
  model_or_process, input_requirements_json, parameters_schema_json,
  protocol_config_json, output_mapping_json, label_mapping_json,
  output_schema_json, display_metadata_json, publish_threshold,
  created_by_user_id, published_by_user_id, published_at
)
select project_id, team_id, definition_id, 1, 'published', 'synchronous', 'demo-ocr-v1',
       '{"type":"object","required":["assetId"]}'::jsonb,
       '{"type":"object","properties":{"language":{"type":"string","title":"识别语言","description":"例如 zh-CN"}}}'::jsonb,
       '{"mappingVersion":"v1"}'::jsonb,
       '{"kind":"ocr","resultPath":"result"}'::jsonb,
       '{}'::jsonb,
       '{"type":"object","properties":{"text":{"type":"string"},"blocks":{"type":"array"}}}'::jsonb,
       jsonb_build_object('helpText', '本地演示输入资产 ID：' || asset_id::text),
       0, user_id, user_id, now()
  from demo
 where not exists (
   select 1 from algorithm_definition_versions version
    where version.algorithm_definition_id = demo.definition_id and version.version = 1
 );

update algorithm_definitions definition
   set current_published_version_id = version.id, updated_at = now()
  from algorithm_definition_versions version, projects project
 where definition.project_id = project.id and project.name = 'DJI Local Demo'
   and definition.name = '通用文档 OCR'
   and version.algorithm_definition_id = definition.id and version.version = 1
   and version.status = 'published'
   and definition.current_published_version_id is distinct from version.id;

update live_streams set status = 'stopped', status_reason = 'LOCAL_DEMO_RESTART',
       ended_at = now(), playback_ref = null, playback_locator_expires_at = null,
       lease_owner = null, lease_expires_at = null, updated_at = now()
 where project_id = (select id from projects where name = 'DJI Local Demo' order by id limit 1)
   and status in ('requested', 'starting', 'live', 'degraded', 'stopping');

commit;

select id from projects where name = 'DJI Local Demo' order by id limit 1;
