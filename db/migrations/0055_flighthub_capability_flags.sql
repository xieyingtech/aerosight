alter table project_feature_flags
  add column flighthub_action_flags_json jsonb not null default '{}'::jsonb,
  add constraint project_feature_flags_flighthub_actions_object
    check (jsonb_typeof(flighthub_action_flags_json) = 'object');
--> statement-breakpoint
update connector_definitions
   set manifest_json = manifest_json || '{
     "writeActionsDefault":"disabled",
     "actionFeatureFlag":"flighthub.actions",
     "capabilities":[
       {"code":"inventory.read","kind":"read","risk":"low"},
       {"code":"state.read","kind":"read","risk":"low","driverCapability":"state.read"},
       {"code":"health.read","kind":"read","risk":"low"},
       {"code":"organization.read","kind":"read","risk":"low"},
       {"code":"flight.read","kind":"read","risk":"low"},
       {"code":"live.read","kind":"read","risk":"low","driverCapability":"stream.video.read"},
       {"code":"geospatial.read","kind":"read","risk":"low"},
       {"code":"model.read","kind":"read","risk":"low"},
       {"code":"security.temporary-credential","kind":"action","risk":"medium","defaultEnabled":false},
       {"code":"flight.execute","kind":"action","risk":"critical","driverCapability":"mission.execute","defaultEnabled":false},
       {"code":"live.control","kind":"action","risk":"high","driverCapability":"stream.video.control","defaultEnabled":false},
       {"code":"geospatial.write","kind":"action","risk":"high","defaultEnabled":false},
       {"code":"model.write","kind":"action","risk":"high","defaultEnabled":false},
       {"code":"device.control","kind":"action","risk":"critical","driverCapability":"flight.return_home","defaultEnabled":false},
       {"code":"organization.write","kind":"action","risk":"critical","defaultEnabled":false}
     ]
   }'::jsonb,
       updated_at = now()
 where connector_key = 'dji.flighthub2' and version = '1.0.0';
