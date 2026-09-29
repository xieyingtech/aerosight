-- Control ownership no longer requires an AeroSight safety policy.
-- Existing policy references are historical metadata, not execution prerequisites.
alter table connector_control_sessions
  alter column safety_policy_version_id drop not null;
