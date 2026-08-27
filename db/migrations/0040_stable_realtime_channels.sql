alter table device_stream_channels add column stable_channel_id text;
--> statement-breakpoint
update device_stream_channels
set stable_channel_id = 'legacy:' || project_id || ':' || device_id || ':' || channel_key
where stable_channel_id is null;
--> statement-breakpoint
create or replace function populate_device_stream_channel_stable_id()
returns trigger language plpgsql as $$
begin
  if new.stable_channel_id is null or length(trim(new.stable_channel_id)) = 0 then
    new.stable_channel_id := 'device:' || new.project_id || ':' || new.device_id || ':' || new.channel_key;
  end if;
  return new;
end;
$$;
--> statement-breakpoint
create trigger device_stream_channels_populate_stable_id
before insert or update of project_id, device_id, channel_key, stable_channel_id
on device_stream_channels for each row execute function populate_device_stream_channel_stable_id();
--> statement-breakpoint
alter table device_stream_channels alter column stable_channel_id set not null;
--> statement-breakpoint
alter table device_stream_channels add constraint device_stream_channels_project_stable_unique
  unique(project_id, stable_channel_id);
--> statement-breakpoint
update driver_definitions
set manifest_json = jsonb_set(manifest_json, '{streams}', '[
  {"channelKey":"telemetry.primary","capabilityCode":"stream.telemetry.read","dataType":"telemetry","unit":"mixed","schema":{"type":"object","properties":{"seq":{"type":"integer"},"latitude":{"type":"number","x-unit":"degree"},"longitude":{"type":"number","x-unit":"degree"},"height":{"type":"number","x-unit":"m"},"horizontal_speed":{"type":"number","x-unit":"m/s"},"vertical_speed":{"type":"number","x-unit":"m/s"}}}},
  {"channelKey":"sensor.primary","capabilityCode":"stream.sensor.read","dataType":"sensor","unit":"mixed","schema":{"type":"object","properties":{"samples":{"type":"object","additionalProperties":{"type":"object","required":["value","unit"],"properties":{"value":{},"unit":{"type":"string"}}}}},"required":["samples"]}},
  {"channelKey":"video.primary","capabilityCode":"stream.video.read","dataType":"video","schema":{"type":"object","properties":{"sessionId":{"type":"string"},"state":{"type":"string"},"playback":{"type":"object"}}}},
  {"channelKey":"events.primary","capabilityCode":"stream.events.read","dataType":"events","schema":{"type":"object"}}
]'::jsonb, true), updated_at=now()
where driver_key='dji.cloud' and version='1.0.0';
