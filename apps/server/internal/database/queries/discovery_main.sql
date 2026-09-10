-- name: DiscoveryList :many
SELECT to_jsonb(r) FROM (
select identity.id::text,identity.adapter_id::text as "connectorId",adapter.name as "connectorName",
             definition.connector_key as "connectorKey",identity.external_device_id as "externalDeviceId",
             identity.external_device_type as "externalDeviceType",
             nullif(identity.identity_json->>'parentExternalId','') as "parentExternalId",
             identity.discovery_status as status,type.type_key as "suggestedTypeKey",
             type.display_name as "suggestedTypeName",identity.match_confidence::float8 as "matchConfidence",
             identity.device_id as "deviceId",identity.last_seen_at::text as "lastSeenAt"
        from device_external_identities identity
        join device_adapters adapter on adapter.id=identity.adapter_id and adapter.project_id=identity.project_id
        join connector_definitions definition on definition.id=adapter.connector_definition_id
        left join device_types type on type.id=identity.suggested_device_type_id
       where identity.project_id=sqlc.arg(p1)
       order by case identity.discovery_status when 'conflicted' then 0 when 'discovered' then 1 when 'missing' then 2 when 'managed' then 3 else 4 end,
                identity.last_seen_at desc,identity.id
) r;

-- name: DiscoveryTypes :many
SELECT to_jsonb(r) FROM (
select id::text,type_key as "typeKey",display_name as "displayName",category
      from device_types where status='active' order by display_name,version desc
) r;

-- name: DiscoveryConnectors :many
SELECT to_jsonb(r) FROM (
select adapter.id::text,adapter.name,definition.connector_key as "connectorKey",adapter.status,
      (definition.connector_key='dji.flighthub2' and definition.status='active' and adapter.status in('connecting','connected','degraded')) as "canScan"
      from device_adapters adapter join connector_definitions definition on definition.id=adapter.connector_definition_id
      where adapter.project_id=sqlc.arg(p1) order by adapter.name
) r;

-- name: DiscoveryLockStatus :many
SELECT to_jsonb(r) FROM (
select discovery_status as status,device_id as "deviceId" from device_external_identities
        where project_id=sqlc.arg(p1) and id=sqlc.arg(p2) for update
) r;

-- name: DiscoveryIgnore :exec
update device_external_identities set discovery_status='ignored'
        where project_id=sqlc.arg(p1) and id=sqlc.arg(p2);

-- name: DiscoveryMatch :many
SELECT to_jsonb(r) FROM (
select type.id::text,type.type_key as "typeKey"
      from device_external_identities identity join device_types type on type.type_key=identity.external_device_type
      where identity.project_id=sqlc.arg(p1) and identity.id=sqlc.arg(p2) and type.status='active'
      order by type.version desc limit 1
) r;

-- name: DiscoveryRematch :exec
update device_external_identities
      set suggested_device_type_id=sqlc.narg(p3),match_confidence=case when sqlc.narg(p3)::bigint is null then null else 1 end,discovery_status=sqlc.arg(p4)
      where project_id=sqlc.arg(p1) and id=sqlc.arg(p2);

-- name: DiscoveryLockBinding :many
SELECT to_jsonb(r) FROM (
select identity.id::text,adapter_id::text as "adapterId",device_id as "deviceId",discovery_status as status,
		nullif(identity.identity_json->>'parentExternalId','') as "parentExternalId",identity.team_id as "teamId",
		adapter.status as "connectorStatus"
		from device_external_identities identity
		join device_adapters adapter on adapter.id=identity.adapter_id and adapter.project_id=identity.project_id
		where identity.project_id=sqlc.arg(p1) and identity.id=sqlc.arg(p2) for update of identity,adapter
) r;

-- name: DiscoveryType :many
SELECT to_jsonb(r) FROM (
select id::text,category from device_types
      where type_key=sqlc.arg(p1) and status='active' order by version desc limit 1
) r;

-- name: DiscoveryLockTarget :many
SELECT to_jsonb(r) FROM (
select id from devices where project_id=sqlc.arg(p1) and id=sqlc.arg(p2) for update
) r;

-- name: DiscoveryStandby :exec
update device_connector_bindings set status='standby'
        where project_id=sqlc.arg(p1) and device_id=sqlc.arg(p2) and status='active';

-- name: DiscoveryCreate :one
insert into devices(
        project_id,adapter_id,device_type_id,name,type,status,metadata_json
      ) values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),sqlc.arg(p4),sqlc.arg(p5),'unknown',sqlc.arg(p6)) returning id;

-- name: DiscoveryUpdateAdapter :exec
update devices set adapter_id=sqlc.arg(p3),updated_at=now() where project_id=sqlc.arg(p1) and id=sqlc.arg(p2);

-- name: DiscoveryBind :exec
update device_external_identities set device_id=sqlc.arg(p3),suggested_device_type_id=sqlc.arg(p4),
      match_confidence=1,discovery_status='managed',bound_at=now()
      where project_id=sqlc.arg(p1) and id=sqlc.arg(p2) and device_id is null;

-- name: DiscoveryRoute :exec
insert into device_connector_bindings(
      project_id,team_id,device_id,connector_instance_id,external_identity_id,route_role,priority,status,metadata_json
    ) values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),sqlc.arg(p4),sqlc.arg(p5),sqlc.arg(p6),
      coalesce((select max(priority)+10 from device_connector_bindings where project_id=sqlc.arg(p1) and device_id=sqlc.arg(p3)),100),
      'active',sqlc.arg(p7))
    on conflict(device_id,connector_instance_id) do update set external_identity_id=excluded.external_identity_id,
      route_role=excluded.route_role,priority=excluded.priority,status='active',unbound_at=null,metadata_json=excluded.metadata_json;

-- name: DiscoveryCapabilities :exec
insert into device_capabilities(
      device_id,project_id,capability_code,version,declared_by_adapter_id,params_schema_json,
      input_schema_json,output_schema_json,risk_level,source_json
    ) select sqlc.arg(p1),sqlc.arg(p2),capability->>'code',driver.version,sqlc.arg(p3),
      coalesce(capability->'inputSchema','{}'),coalesce(capability->'inputSchema','{}'),
      coalesce(capability->'outputSchema','{}'),coalesce(capability->>'risk','low'),
      jsonb_build_object('driver',driver.driver_key,'typeKey',type.type_key)
      from device_types type join driver_definitions driver on driver.id=type.driver_definition_id
      cross join lateral jsonb_array_elements(case when jsonb_typeof(driver.manifest_json->'capabilities')='array'
        then driver.manifest_json->'capabilities' else '[]'::jsonb end) capability
      where type.id=sqlc.arg(p4) and type.capability_profile_json ? (capability->>'code')
      on conflict(device_id,capability_code) do update set declared_by_adapter_id=excluded.declared_by_adapter_id,
        availability='available',availability_reason=null,updated_at=now();

-- name: DiscoveryRelationship :exec
insert into device_relationships(
      project_id,team_id,from_device_id,to_device_id,relation_type,source_type,metadata_json
    ) select sqlc.arg(p1),sqlc.arg(p2),parent.device_id,sqlc.arg(p3),'contains','discovery',sqlc.arg(p6)
      from device_external_identities parent where parent.project_id=sqlc.arg(p1) and parent.adapter_id=sqlc.arg(p4)
        and parent.external_device_id=sqlc.arg(p5) and parent.device_id is not null
        and not exists(select 1 from device_relationships relation where relation.project_id=sqlc.arg(p1)
          and relation.from_device_id=parent.device_id and relation.to_device_id=sqlc.arg(p3) and relation.valid_until is null);
