-- Model capabilities are the source for operation discovery. This declares
-- applicability only, without grants, feature switches or verification evidence.
update driver_definitions set manifest_json=jsonb_set(manifest_json,'{capabilities}',
  (select coalesce(jsonb_agg(item),'[]'::jsonb) from jsonb_array_elements(case when jsonb_typeof(manifest_json->'capabilities')='array' then manifest_json->'capabilities' else '[]'::jsonb end) item
   where item->>'code' not in('camera.payload.control','stream.video.quality')) || '[
  {"code":"camera.payload.control","kind":"command","risk":"medium","inputSchema":{"type":"object"}},
  {"code":"stream.video.quality","kind":"command","risk":"low","inputSchema":{"type":"object"}}
  ]'::jsonb),updated_at=now()
where driver_key='dji.cloud' and version='1.0.0';
--> statement-breakpoint
update device_types set capability_profile_json=capability_profile_json||'{"stream.video.quality":{"enabled":true}}'::jsonb,updated_at=now()
where type_key in('dji.dock2','dji.matrice3d','dji.matrice3td');
--> statement-breakpoint
update device_types set capability_profile_json=capability_profile_json||'{"camera.payload.control":{"enabled":true}}'::jsonb,updated_at=now()
where type_key in('dji.matrice3d','dji.matrice3td');
--> statement-breakpoint
insert into device_capabilities(device_id,project_id,device_type_id,driver_definition_id,declared_by_adapter_id,capability_code,risk_level,input_schema_json,source_json)
select device.id,device.project_id,type.id,driver.id,binding.connector_instance_id,definition->>'code',definition->>'risk',definition->'inputSchema',
 jsonb_build_object('driver',driver.driver_key,'typeKey',type.type_key)
from devices device join device_types type on type.id=device.device_type_id
join driver_definitions driver on driver.id=type.driver_definition_id
join lateral(select route.connector_instance_id from device_connector_bindings route
 join device_adapters adapter on adapter.id=route.connector_instance_id
 join connector_definitions connector on connector.id=adapter.connector_definition_id
 where route.device_id=device.id and route.project_id=device.project_id and route.status='active' and connector.connector_key='dji.flighthub2'
 order by route.priority desc limit 1) binding on true
cross join lateral jsonb_array_elements(case when jsonb_typeof(driver.manifest_json->'capabilities')='array' then driver.manifest_json->'capabilities' else '[]'::jsonb end) definition
where definition->>'code' in('camera.payload.control','stream.video.quality')
 and type.capability_profile_json->(definition->>'code')->>'enabled'='true'
on conflict(device_id,capability_code) do nothing;
