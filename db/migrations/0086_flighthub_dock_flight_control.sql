-- Device applicability only; this does not grant permission or field verification.
update device_types set capability_profile_json=capability_profile_json||'{"flight.return_home":{"enabled":true}}'::jsonb
where type_key='dji.dock2';
--> statement-breakpoint
insert into device_capabilities(device_id,project_id,device_type_id,driver_definition_id,capability_code,risk_level,input_schema_json)
select device.id,device.project_id,device.device_type_id,type.driver_definition_id,'flight.return_home','critical','{"type":"object"}'::jsonb
from devices device join device_types type on type.id=device.device_type_id
where type.type_key='dji.dock2' and type.driver_definition_id is not null
and exists(select 1 from device_connector_bindings binding join device_adapters adapter on adapter.id=binding.connector_instance_id
 join connector_definitions definition on definition.id=adapter.connector_definition_id
 where binding.project_id=device.project_id and binding.device_id=device.id and binding.status='active' and definition.connector_key='dji.flighthub2')
on conflict(device_id,capability_code) do nothing;
