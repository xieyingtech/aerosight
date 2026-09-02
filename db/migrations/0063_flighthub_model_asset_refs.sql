alter table connector_asset_access_refs
  drop constraint connector_asset_access_refs_kind_valid;
--> statement-breakpoint
alter table connector_asset_access_refs
  add constraint connector_asset_access_refs_kind_valid
    check(access_kind in('flight-media','flight-record','model','model-resource'));
