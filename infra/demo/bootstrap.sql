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

update live_streams set status = 'stopped', status_reason = 'LOCAL_DEMO_RESTART',
       ended_at = now(), playback_ref = null, playback_locator_expires_at = null,
       lease_owner = null, lease_expires_at = null, updated_at = now()
 where project_id = (select id from projects where name = 'DJI Local Demo' order by id limit 1)
   and status in ('requested', 'starting', 'live', 'degraded', 'stopping');

commit;

select id from projects where name = 'DJI Local Demo' order by id limit 1;
