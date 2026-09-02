alter table connector_remote_resources
  drop constraint if exists connector_remote_resources_kind_valid;
--> statement-breakpoint
alter table connector_remote_resources
  add constraint connector_remote_resources_kind_valid
  check (resource_kind in (
    'wayline','flight-task','flight-media','flight-record','flight-alert','ai-alert',
    'map-element','flight-area','offline-map','air-sense-warning','model','model-resource',
    'live-share','stream-converter','recording','hms','topology','auto-record',
    'organization','organization-user','organization-role','organization-permission',
    'project-user','project-member'
  ));
