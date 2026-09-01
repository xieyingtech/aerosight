insert into connector_definitions (connector_key, version, display_name, status, manifest_json)
values (
  'dji.flighthub2',
  '1.0.0',
  'DJI FlightHub 2',
  'active',
  '{
    "discoveryModes":["poll"],
    "protocols":["https"],
    "compatibleDrivers":["dji.cloud"],
    "readOnly":true,
    "capabilities":["inventory.read","state.read"],
    "configSchema":{
      "type":"object",
      "additionalProperties":false,
      "properties":{}
    },
    "credentialSchema":{
      "type":"object",
      "additionalProperties":false,
      "required":["token"],
      "properties":{
        "token":{"type":"string","minLength":1,"writeOnly":true}
      }
    },
    "discoveryScopeSchema":{
      "type":"object",
      "additionalProperties":false,
      "required":["projectUuid","projectName"],
      "properties":{
        "projectUuid":{"type":"string","format":"uuid"},
        "projectName":{"type":"string","minLength":1}
      }
    }
  }'::jsonb
)
on conflict (connector_key, version) do nothing;
