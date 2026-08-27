insert into driver_definitions (driver_key, version, display_name, manifest_json)
values (
  'dji.cloud', '1.0.0', 'DJI Cloud API',
  '{
    "driverKey":"dji.cloud","version":"1.0.0","displayName":"DJI Cloud API",
    "protocols":["mqtt5"],
    "capabilities":[
      {"code":"state.read","kind":"read","risk":"low","outputSchema":{"type":"object"}},
      {"code":"mission.execute","kind":"command","risk":"high","inputSchema":{"type":"object"}},
      {"code":"mission.cancel","kind":"command","risk":"high","inputSchema":{"type":"object"}},
      {"code":"flight.return_home","kind":"command","risk":"critical","inputSchema":{"type":"object"}},
      {"code":"stream.video.control","kind":"command","risk":"medium","inputSchema":{"type":"object"}},
      {"code":"dock.debug.control","kind":"command","risk":"critical","inputSchema":{"type":"object"}},
      {"code":"stream.telemetry.read","kind":"stream","risk":"low","outputSchema":{"type":"object"}},
      {"code":"stream.sensor.read","kind":"stream","risk":"low","outputSchema":{"type":"object"}},
      {"code":"stream.video.read","kind":"stream","risk":"low","outputSchema":{"type":"object"}},
      {"code":"stream.events.read","kind":"stream","risk":"low","outputSchema":{"type":"object"}}
    ],
    "streams":[
      {"channelKey":"telemetry.primary","capabilityCode":"stream.telemetry.read","dataType":"telemetry","schema":{"type":"object"}},
      {"channelKey":"sensor.primary","capabilityCode":"stream.sensor.read","dataType":"sensor","schema":{"type":"object"}},
      {"channelKey":"video.primary","capabilityCode":"stream.video.read","dataType":"video","schema":{"type":"object"}},
      {"channelKey":"events.primary","capabilityCode":"stream.events.read","dataType":"events","schema":{"type":"object"}}
    ]
  }'::jsonb
)
on conflict (driver_key, version) do update
set display_name = excluded.display_name, manifest_json = excluded.manifest_json, updated_at = now();
--> statement-breakpoint
with driver as (
  select id from driver_definitions where driver_key = 'dji.cloud' and version = '1.0.0'
), catalog(type_key, display_name, category, model, capability_profile) as (
  values
    ('dji.unknown', 'Unknown DJI device', 'unknown', null, '{"state.read":{"enabled":true,"diagnosticOnly":true}}'::jsonb),
    ('dji.dock2', 'DJI Dock 2', 'dock', 'Dock 2', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"dock.debug.control":{"enabled":true,"productFamily":"dock2"}}'::jsonb),
    ('dji.matrice3d', 'DJI Matrice 3D', 'aircraft', 'Matrice 3D', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"flight.return_home":{"enabled":true},"stream.telemetry.read":{"enabled":true}}'::jsonb),
    ('dji.matrice3td', 'DJI Matrice 3TD', 'aircraft', 'Matrice 3TD', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"flight.return_home":{"enabled":true},"stream.telemetry.read":{"enabled":true}}'::jsonb),
    ('dji.matrice3d.camera', 'Matrice 3D Camera', 'camera', 'Matrice 3D Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true}}'::jsonb),
    ('dji.matrice3td.camera', 'Matrice 3TD Camera', 'camera', 'Matrice 3TD Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true},"stream.sensor.read":{"enabled":true}}'::jsonb),
    ('dji.matrice3.vision-assist', 'Matrice 3 Vision Assist', 'camera', 'Matrice 3 Vision Assist', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true}}'::jsonb),
    ('dji.dock2.camera', 'DJI Dock 2 Camera', 'camera', 'Dock 2 Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true}}'::jsonb),
    ('dji.dock2.environment-sensor', 'DJI Dock 2 Environment Sensor', 'sensor', 'Dock 2 Environment Sensor', '{"state.read":{"enabled":true},"stream.sensor.read":{"enabled":true}}'::jsonb),
    ('dji.dock3', 'DJI Dock 3', 'dock', 'Dock 3', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"dock.debug.control":{"enabled":true,"productFamily":"dock3"}}'::jsonb),
    ('dji.matrice4d', 'DJI Matrice 4D', 'aircraft', 'Matrice 4D', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"flight.return_home":{"enabled":true},"stream.telemetry.read":{"enabled":true}}'::jsonb),
    ('dji.matrice4td', 'DJI Matrice 4TD', 'aircraft', 'Matrice 4TD', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"flight.return_home":{"enabled":true},"stream.telemetry.read":{"enabled":true}}'::jsonb),
    ('dji.matrice4d.camera', 'Matrice 4D Camera', 'camera', 'Matrice 4D Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true}}'::jsonb),
    ('dji.matrice4td.camera', 'Matrice 4TD Camera', 'camera', 'Matrice 4TD Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true},"stream.sensor.read":{"enabled":true}}'::jsonb),
    ('dji.matrice4.vision-assist', 'Matrice 4 Vision Assist', 'camera', 'Matrice 4 Vision Assist', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true}}'::jsonb),
    ('dji.dock3.camera', 'DJI Dock 3 Camera', 'camera', 'Dock 3 Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true}}'::jsonb),
    ('dji.dock3.environment-sensor', 'DJI Dock 3 Environment Sensor', 'sensor', 'Dock 3 Environment Sensor', '{"state.read":{"enabled":true},"stream.sensor.read":{"enabled":true}}'::jsonb)
)
insert into device_types (
  type_key, version, display_name, category, vendor, model,
  driver_definition_id, driver_version_constraint, capability_profile_json
)
select catalog.type_key, 1, catalog.display_name, catalog.category, 'dji', catalog.model,
       driver.id, '^1.0.0', catalog.capability_profile
from catalog cross join driver
on conflict (type_key, version) do update
set display_name = excluded.display_name, category = excluded.category, vendor = excluded.vendor,
    model = excluded.model, driver_definition_id = excluded.driver_definition_id,
    driver_version_constraint = excluded.driver_version_constraint,
    capability_profile_json = excluded.capability_profile_json, updated_at = now();
