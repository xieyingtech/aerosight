alter table connector_capability_snapshots
  add column account_fingerprint text;
--> statement-breakpoint
alter table connector_capability_snapshots
  add constraint connector_capability_snapshots_account_fingerprint_valid
    check (account_fingerprint is null or account_fingerprint ~ '^[a-f0-9]{64}$');
--> statement-breakpoint
drop index connector_capability_snapshots_identity_unique;
--> statement-breakpoint
create unique index connector_capability_snapshots_identity_unique
  on connector_capability_snapshots(
    project_id, connector_instance_id, capability_code, region, deployment,
    account_fingerprint, device_model, firmware_version
  ) nulls not distinct;
--> statement-breakpoint
create index connector_capability_snapshots_acceptance_scope_idx
  on connector_capability_snapshots(
    connector_instance_id, account_fingerprint, capability_code, status, expires_at
  ) where evidence_level='field-write';
