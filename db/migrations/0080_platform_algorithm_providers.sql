-- Algorithm connections are shared platform resources. Retain legacy project_id
-- only as the encryption scope for credentials written before this migration.
ALTER TABLE algorithm_definitions DROP CONSTRAINT algorithm_definitions_provider_project_fk;
ALTER TABLE algorithm_callback_receipts DROP CONSTRAINT algorithm_callback_receipts_provider_project_fk;
ALTER TABLE algorithm_providers DROP CONSTRAINT algorithm_providers_project_team_fk;
ALTER TABLE algorithm_providers ALTER COLUMN project_id DROP NOT NULL;
ALTER TABLE algorithm_providers ALTER COLUMN team_id DROP NOT NULL;
ALTER TABLE algorithm_providers DROP CONSTRAINT algorithm_providers_project_name_unique;
ALTER TABLE algorithm_definitions ADD CONSTRAINT algorithm_definitions_provider_fk
  FOREIGN KEY (provider_id) REFERENCES algorithm_providers(id) ON DELETE RESTRICT;
ALTER TABLE algorithm_callback_receipts ADD CONSTRAINT algorithm_callback_receipts_provider_fk
  FOREIGN KEY (provider_id) REFERENCES algorithm_providers(id) ON DELETE RESTRICT;
