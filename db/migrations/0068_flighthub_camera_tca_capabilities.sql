update driver_definitions
   set manifest_json=jsonb_set(
     manifest_json,'{capabilities}',coalesce(manifest_json->'capabilities','[]'::jsonb)||'[
       {"code":"camera.change","kind":"command","risk":"high","inputSchema":{"type":"object","additionalProperties":false,"required":["cameraIndex"],"properties":{"cameraIndex":{"type":"string"},"cameraPosition":{"type":"string"}}}},
       {"code":"camera.lens.change","kind":"command","risk":"high","inputSchema":{"type":"object","additionalProperties":false,"required":["cameraIndex","lensType"],"properties":{"cameraIndex":{"type":"string"},"lensType":{"type":"string"}}}}
     ]'::jsonb,true),updated_at=now()
 where driver_key='dji.cloud' and version='1.0.0';
--> statement-breakpoint
update device_types
   set capability_profile_json=capability_profile_json||'{"camera.change":{"enabled":true}}'::jsonb,updated_at=now()
 where type_key in('dji.dock2','dji.dock3');
--> statement-breakpoint
update device_types
   set capability_profile_json=capability_profile_json||'{"camera.lens.change":{"enabled":true}}'::jsonb,updated_at=now()
 where type_key in('dji.matrice3d','dji.matrice3td','dji.matrice4d','dji.matrice4td');
--> statement-breakpoint
insert into device_capabilities(
  device_id,project_id,capability_code,version,declared_by_adapter_id,params_schema_json,
  input_schema_json,output_schema_json,risk_level,source_json
)
select device.id,device.project_id,capability->>'code',driver.version,route.connector_instance_id,
  coalesce(capability->'inputSchema','{}'::jsonb),coalesce(capability->'inputSchema','{}'::jsonb),
  coalesce(capability->'outputSchema','{}'::jsonb),coalesce(capability->>'risk','low'),
  jsonb_build_object('driver',driver.driver_key,'typeKey',device_type.type_key)
from devices device
join device_types device_type on device_type.id=device.device_type_id
join driver_definitions driver on driver.id=device_type.driver_definition_id
join lateral(
  select binding.connector_instance_id
    from device_connector_bindings binding
    join device_adapters adapter on adapter.id=binding.connector_instance_id and adapter.project_id=binding.project_id
    join connector_definitions definition on definition.id=adapter.connector_definition_id
   where binding.project_id=device.project_id and binding.device_id=device.id and binding.status='active'
     and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
   order by binding.priority desc,binding.connector_instance_id limit 1
) route on true
cross join lateral jsonb_array_elements(case when jsonb_typeof(driver.manifest_json->'capabilities')='array'
  then driver.manifest_json->'capabilities' else '[]'::jsonb end) capability
where capability->>'code' in('camera.change','camera.lens.change')
  and device_type.capability_profile_json ? (capability->>'code')
on conflict(device_id,capability_code) do update
set declared_by_adapter_id=excluded.declared_by_adapter_id,params_schema_json=excluded.params_schema_json,
    input_schema_json=excluded.input_schema_json,output_schema_json=excluded.output_schema_json,
    risk_level=excluded.risk_level,availability='available',availability_reason=null,
    source_json=excluded.source_json,updated_at=now();
--> statement-breakpoint
update connector_definitions
   set manifest_json=jsonb_set(
     manifest_json,'{capabilities}',coalesce(manifest_json->'capabilities','[]'::jsonb)||'[
       {"code":"device.camera.change","kind":"action","risk":"high","driverCapability":"camera.change","featureFlag":"flighthub.camera.change","defaultEnabled":false},
       {"code":"device.lens.change","kind":"action","risk":"high","driverCapability":"camera.lens.change","featureFlag":"flighthub.lens.change","defaultEnabled":false},
       {"code":"tca.status.read","kind":"read","risk":"low","defaultEnabled":false}
     ]'::jsonb,true),updated_at=now()
 where connector_key='dji.flighthub2' and version='1.0.0';
