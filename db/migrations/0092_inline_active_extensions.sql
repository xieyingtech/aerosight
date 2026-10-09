-- Inline active one-to-one extensions. Views retain the legacy SQL contract without duplicate storage.

LOCK TABLE device_commands, assets, observations, device_command_protocol_correlations, connector_asset_access_refs, poses IN ACCESS EXCLUSIVE MODE;

ALTER TABLE device_commands
  ADD COLUMN protocol_correlation_id bigint,
  ADD COLUMN protocol_adapter_id bigint,
  ADD COLUMN protocol_mapping_version text,
  ADD COLUMN protocol_transaction_id text,
  ADD COLUMN protocol_business_id text,
  ADD COLUMN protocol_method text,
  ADD COLUMN protocol_request_topic text,
  ADD COLUMN protocol_request_payload_json jsonb,
  ADD COLUMN protocol_status text,
  ADD COLUMN protocol_reply_event_id text,
  ADD COLUMN protocol_reply_result integer,
  ADD COLUMN protocol_reply_payload_json jsonb,
  ADD COLUMN protocol_sent_at timestamptz,
  ADD COLUMN protocol_replied_at timestamptz,
  ADD COLUMN protocol_created_at timestamptz,
  ADD COLUMN protocol_updated_at timestamptz;

UPDATE device_commands p SET
  protocol_correlation_id = c.id,
  protocol_adapter_id = c.adapter_id,
  protocol_mapping_version = c.mapping_version,
  protocol_transaction_id = c.transaction_id,
  protocol_business_id = c.business_id,
  protocol_method = c.method,
  protocol_request_topic = c.request_topic,
  protocol_request_payload_json = c.request_payload_json,
  protocol_status = c.status,
  protocol_reply_event_id = c.reply_event_id,
  protocol_reply_result = c.reply_result,
  protocol_reply_payload_json = c.reply_payload_json,
  protocol_sent_at = c.sent_at,
  protocol_replied_at = c.replied_at,
  protocol_created_at = c.created_at,
  protocol_updated_at = c.updated_at
FROM device_command_protocol_correlations c WHERE p.id = c.command_id AND p.project_id = c.project_id;

DO $$ BEGIN IF (SELECT count(*) FROM device_command_protocol_correlations) <> (SELECT count(*) FROM device_commands WHERE protocol_status IS NOT NULL) THEN RAISE EXCEPTION 'device_command_protocol_correlations migration lost rows'; END IF; END $$;

ALTER TABLE device_commands ADD CONSTRAINT device_commands_protocol_shape_check CHECK ((protocol_correlation_id IS NULL AND protocol_adapter_id IS NULL AND protocol_mapping_version IS NULL AND protocol_transaction_id IS NULL AND protocol_business_id IS NULL AND protocol_method IS NULL AND protocol_request_topic IS NULL AND protocol_request_payload_json IS NULL AND protocol_status IS NULL AND protocol_reply_event_id IS NULL AND protocol_reply_result IS NULL AND protocol_reply_payload_json IS NULL AND protocol_sent_at IS NULL AND protocol_replied_at IS NULL AND protocol_created_at IS NULL AND protocol_updated_at IS NULL) OR (protocol_correlation_id IS NOT NULL AND protocol_adapter_id IS NOT NULL AND protocol_mapping_version IS NOT NULL AND protocol_transaction_id IS NOT NULL AND protocol_business_id IS NOT NULL AND protocol_method IS NOT NULL AND protocol_request_topic IS NOT NULL AND protocol_request_payload_json IS NOT NULL AND protocol_status IS NOT NULL AND protocol_created_at IS NOT NULL AND protocol_updated_at IS NOT NULL AND protocol_status IN ('prepared','sent','acknowledged','nacked','unknown')));

ALTER TABLE device_commands ADD CONSTRAINT device_commands_protocol_adapter_id_fkey FOREIGN KEY (protocol_adapter_id,project_id) REFERENCES device_adapters(id,project_id) ON DELETE SET NULL (protocol_adapter_id);

CREATE FUNCTION clear_device_commands_protocol_extension() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.protocol_status IS NOT NULL AND (NEW.protocol_adapter_id IS NULL) THEN
    NEW.protocol_correlation_id := NULL;
    NEW.protocol_adapter_id := NULL;
    NEW.protocol_mapping_version := NULL;
    NEW.protocol_transaction_id := NULL;
    NEW.protocol_business_id := NULL;
    NEW.protocol_method := NULL;
    NEW.protocol_request_topic := NULL;
    NEW.protocol_request_payload_json := NULL;
    NEW.protocol_status := NULL;
    NEW.protocol_reply_event_id := NULL;
    NEW.protocol_reply_result := NULL;
    NEW.protocol_reply_payload_json := NULL;
    NEW.protocol_sent_at := NULL;
    NEW.protocol_replied_at := NULL;
    NEW.protocol_created_at := NULL;
    NEW.protocol_updated_at := NULL;
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER clear_device_commands_protocol_extension BEFORE UPDATE ON device_commands FOR EACH ROW EXECUTE FUNCTION clear_device_commands_protocol_extension();

ALTER SEQUENCE device_command_protocol_correlations_id_seq OWNED BY device_commands.protocol_correlation_id;

ALTER SEQUENCE device_command_protocol_correlations_id_seq RENAME TO device_commands_protocol_correlation_id_seq;

SELECT setval('device_commands_protocol_correlation_id_seq', GREATEST((SELECT last_value FROM device_commands_protocol_correlation_id_seq), COALESCE((SELECT max(protocol_correlation_id) FROM device_commands),1)),true);

DROP TABLE device_command_protocol_correlations;

CREATE VIEW device_command_protocol_correlations AS SELECT protocol_correlation_id AS id,
  project_id,
  team_id,
  id AS command_id,
  protocol_adapter_id AS adapter_id,
  protocol_mapping_version AS mapping_version,
  protocol_transaction_id AS transaction_id,
  protocol_business_id AS business_id,
  protocol_method AS method,
  protocol_request_topic AS request_topic,
  protocol_request_payload_json AS request_payload_json,
  protocol_status AS status,
  protocol_reply_event_id AS reply_event_id,
  protocol_reply_result AS reply_result,
  protocol_reply_payload_json AS reply_payload_json,
  protocol_sent_at AS sent_at,
  protocol_replied_at AS replied_at,
  protocol_created_at AS created_at,
  protocol_updated_at AS updated_at
FROM device_commands WHERE protocol_status IS NOT NULL;

CREATE FUNCTION write_device_command_protocol_correlations_extension() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    UPDATE device_commands SET protocol_correlation_id = NULL, protocol_adapter_id = NULL, protocol_mapping_version = NULL, protocol_transaction_id = NULL, protocol_business_id = NULL, protocol_method = NULL, protocol_request_topic = NULL, protocol_request_payload_json = NULL, protocol_status = NULL, protocol_reply_event_id = NULL, protocol_reply_result = NULL, protocol_reply_payload_json = NULL, protocol_sent_at = NULL, protocol_replied_at = NULL, protocol_created_at = NULL, protocol_updated_at = NULL WHERE id = OLD.command_id AND project_id = OLD.project_id AND team_id = OLD.team_id;
    RETURN OLD;
  END IF;
  IF TG_OP = 'UPDATE' AND (NEW.command_id IS DISTINCT FROM OLD.command_id OR NEW.project_id IS DISTINCT FROM OLD.project_id OR NEW.team_id IS DISTINCT FROM OLD.team_id OR NEW.id IS DISTINCT FROM OLD.id) THEN
    RAISE EXCEPTION 'extension identity cannot change' USING ERRCODE = '23514';
  END IF;
  IF TG_OP = 'INSERT' THEN
    NEW.id := COALESCE(NEW.id,nextval('device_commands_protocol_correlation_id_seq'));
    NEW.status := COALESCE(NEW.status,'prepared');
    NEW.created_at := COALESCE(NEW.created_at,now());
    NEW.updated_at := COALESCE(NEW.updated_at,now());
    IF NOT EXISTS (SELECT 1 FROM device_commands WHERE id = NEW.command_id AND project_id = NEW.project_id AND team_id = NEW.team_id) THEN
      RAISE EXCEPTION 'extension parent does not exist in scope' USING ERRCODE = '23503';
    END IF;
    UPDATE device_commands SET protocol_correlation_id = NEW.id,
    protocol_adapter_id = NEW.adapter_id,
    protocol_mapping_version = NEW.mapping_version,
    protocol_transaction_id = NEW.transaction_id,
    protocol_business_id = NEW.business_id,
    protocol_method = NEW.method,
    protocol_request_topic = NEW.request_topic,
    protocol_request_payload_json = NEW.request_payload_json,
    protocol_status = NEW.status,
    protocol_reply_event_id = NEW.reply_event_id,
    protocol_reply_result = NEW.reply_result,
    protocol_reply_payload_json = NEW.reply_payload_json,
    protocol_sent_at = NEW.sent_at,
    protocol_replied_at = NEW.replied_at,
    protocol_created_at = NEW.created_at,
    protocol_updated_at = NEW.updated_at WHERE id = NEW.command_id AND project_id = NEW.project_id AND team_id = NEW.team_id AND protocol_status IS NULL;
    IF NOT FOUND THEN
      RAISE EXCEPTION 'extension already exists' USING ERRCODE = '23505';
    END IF;
  ELSE
    UPDATE device_commands SET protocol_correlation_id = NEW.id,
    protocol_adapter_id = NEW.adapter_id,
    protocol_mapping_version = NEW.mapping_version,
    protocol_transaction_id = NEW.transaction_id,
    protocol_business_id = NEW.business_id,
    protocol_method = NEW.method,
    protocol_request_topic = NEW.request_topic,
    protocol_request_payload_json = NEW.request_payload_json,
    protocol_status = NEW.status,
    protocol_reply_event_id = NEW.reply_event_id,
    protocol_reply_result = NEW.reply_result,
    protocol_reply_payload_json = NEW.reply_payload_json,
    protocol_sent_at = NEW.sent_at,
    protocol_replied_at = NEW.replied_at,
    protocol_created_at = NEW.created_at,
    protocol_updated_at = NEW.updated_at WHERE id = OLD.command_id AND project_id = OLD.project_id AND team_id = OLD.team_id;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER write_device_command_protocol_correlations_extension INSTEAD OF INSERT OR UPDATE OR DELETE ON device_command_protocol_correlations
FOR EACH ROW EXECUTE FUNCTION write_device_command_protocol_correlations_extension();

ALTER TABLE assets
  ADD COLUMN remote_connector_id bigint,
  ADD COLUMN remote_resource_id bigint,
  ADD COLUMN remote_access_kind text,
  ADD COLUMN remote_reference_digest text,
  ADD COLUMN remote_credential_envelope_json jsonb,
  ADD COLUMN remote_reference_created_at timestamptz,
  ADD COLUMN remote_reference_updated_at timestamptz;

UPDATE assets p SET
  remote_connector_id = c.connector_instance_id,
  remote_resource_id = c.remote_resource_id,
  remote_access_kind = c.access_kind,
  remote_reference_digest = c.reference_digest,
  remote_credential_envelope_json = c.credential_envelope_json,
  remote_reference_created_at = c.created_at,
  remote_reference_updated_at = c.updated_at
FROM connector_asset_access_refs c WHERE p.id = c.id AND p.project_id = c.project_id;

DO $$ BEGIN IF (SELECT count(*) FROM connector_asset_access_refs) <> (SELECT count(*) FROM assets WHERE remote_access_kind IS NOT NULL) THEN RAISE EXCEPTION 'connector_asset_access_refs migration lost rows'; END IF; END $$;

ALTER TABLE assets ADD CONSTRAINT assets_remote_shape_check CHECK ((remote_connector_id IS NULL AND remote_resource_id IS NULL AND remote_access_kind IS NULL AND remote_reference_digest IS NULL AND remote_credential_envelope_json IS NULL AND remote_reference_created_at IS NULL AND remote_reference_updated_at IS NULL) OR (remote_connector_id IS NOT NULL AND remote_resource_id IS NOT NULL AND remote_access_kind IS NOT NULL AND remote_reference_digest IS NOT NULL AND remote_credential_envelope_json IS NOT NULL AND remote_reference_created_at IS NOT NULL AND remote_reference_updated_at IS NOT NULL AND remote_access_kind IN ('flight-media','flight-record','model','model-resource') AND remote_reference_digest ~ '^[a-f0-9]{64}$' AND jsonb_typeof(remote_credential_envelope_json) = 'object'));

ALTER TABLE assets ADD CONSTRAINT assets_remote_connector_id_fkey FOREIGN KEY (remote_connector_id,project_id) REFERENCES device_adapters(id,project_id) ON DELETE SET NULL (remote_connector_id);

ALTER TABLE assets ADD CONSTRAINT assets_remote_resource_id_fkey FOREIGN KEY (remote_resource_id,project_id) REFERENCES connector_remote_resources(id,project_id) ON DELETE SET NULL (remote_resource_id);

CREATE FUNCTION clear_assets_remote_extension() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.remote_access_kind IS NOT NULL AND (NEW.remote_connector_id IS NULL OR NEW.remote_resource_id IS NULL) THEN
    NEW.remote_connector_id := NULL;
    NEW.remote_resource_id := NULL;
    NEW.remote_access_kind := NULL;
    NEW.remote_reference_digest := NULL;
    NEW.remote_credential_envelope_json := NULL;
    NEW.remote_reference_created_at := NULL;
    NEW.remote_reference_updated_at := NULL;
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER clear_assets_remote_extension BEFORE UPDATE ON assets FOR EACH ROW EXECUTE FUNCTION clear_assets_remote_extension();

DROP TABLE connector_asset_access_refs;

CREATE VIEW connector_asset_access_refs AS SELECT id AS id,
  project_id,
  team_id,
  remote_connector_id AS connector_instance_id,
  remote_resource_id AS remote_resource_id,
  remote_access_kind AS access_kind,
  remote_reference_digest AS reference_digest,
  remote_credential_envelope_json AS credential_envelope_json,
  remote_reference_created_at AS created_at,
  remote_reference_updated_at AS updated_at
FROM assets WHERE remote_access_kind IS NOT NULL;

CREATE FUNCTION write_connector_asset_access_refs_extension() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    UPDATE assets SET remote_connector_id = NULL, remote_resource_id = NULL, remote_access_kind = NULL, remote_reference_digest = NULL, remote_credential_envelope_json = NULL, remote_reference_created_at = NULL, remote_reference_updated_at = NULL WHERE id = OLD.id AND project_id = OLD.project_id AND team_id = OLD.team_id;
    RETURN OLD;
  END IF;
  IF TG_OP = 'UPDATE' AND (NEW.id IS DISTINCT FROM OLD.id OR NEW.project_id IS DISTINCT FROM OLD.project_id OR NEW.team_id IS DISTINCT FROM OLD.team_id) THEN
    RAISE EXCEPTION 'extension identity cannot change' USING ERRCODE = '23514';
  END IF;
  IF TG_OP = 'INSERT' THEN
    NEW.created_at := COALESCE(NEW.created_at,now());
    NEW.updated_at := COALESCE(NEW.updated_at,now());
    IF NOT EXISTS (SELECT 1 FROM assets WHERE id = NEW.id AND project_id = NEW.project_id AND team_id = NEW.team_id) THEN
      RAISE EXCEPTION 'extension parent does not exist in scope' USING ERRCODE = '23503';
    END IF;
    UPDATE assets SET remote_connector_id = NEW.connector_instance_id,
    remote_resource_id = NEW.remote_resource_id,
    remote_access_kind = NEW.access_kind,
    remote_reference_digest = NEW.reference_digest,
    remote_credential_envelope_json = NEW.credential_envelope_json,
    remote_reference_created_at = NEW.created_at,
    remote_reference_updated_at = NEW.updated_at WHERE id = NEW.id AND project_id = NEW.project_id AND team_id = NEW.team_id AND remote_access_kind IS NULL;
    IF NOT FOUND THEN
      RAISE EXCEPTION 'extension already exists' USING ERRCODE = '23505';
    END IF;
  ELSE
    UPDATE assets SET remote_connector_id = NEW.connector_instance_id,
    remote_resource_id = NEW.remote_resource_id,
    remote_access_kind = NEW.access_kind,
    remote_reference_digest = NEW.reference_digest,
    remote_credential_envelope_json = NEW.credential_envelope_json,
    remote_reference_created_at = NEW.created_at,
    remote_reference_updated_at = NEW.updated_at WHERE id = OLD.id AND project_id = OLD.project_id AND team_id = OLD.team_id;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER write_connector_asset_access_refs_extension INSTEAD OF INSERT OR UPDATE OR DELETE ON connector_asset_access_refs
FOR EACH ROW EXECUTE FUNCTION write_connector_asset_access_refs_extension();

ALTER TABLE observations
  ADD COLUMN pose_device_id integer,
  ADD COLUMN pose_captured_at timestamptz,
  ADD COLUMN pose_standard_position geometry(PointZ,4326),
  ADD COLUMN pose_original_position geometry(PointZ),
  ADD COLUMN pose_orientation_x double precision,
  ADD COLUMN pose_orientation_y double precision,
  ADD COLUMN pose_orientation_z double precision,
  ADD COLUMN pose_orientation_w double precision,
  ADD COLUMN pose_velocity_x double precision,
  ADD COLUMN pose_velocity_y double precision,
  ADD COLUMN pose_velocity_z double precision,
  ADD COLUMN pose_horizontal_accuracy_m double precision,
  ADD COLUMN pose_vertical_accuracy_m double precision,
  ADD COLUMN pose_attitude_accuracy_deg double precision,
  ADD COLUMN pose_vertical_datum text,
  ADD COLUMN pose_transform_version text,
  ADD COLUMN pose_spatial_quality text;

UPDATE observations p SET
  pose_device_id = c.device_id,
  pose_captured_at = c.captured_at,
  pose_standard_position = c.standard_position,
  pose_original_position = c.original_position,
  pose_orientation_x = c.orientation_x,
  pose_orientation_y = c.orientation_y,
  pose_orientation_z = c.orientation_z,
  pose_orientation_w = c.orientation_w,
  pose_velocity_x = c.velocity_x,
  pose_velocity_y = c.velocity_y,
  pose_velocity_z = c.velocity_z,
  pose_horizontal_accuracy_m = c.horizontal_accuracy_m,
  pose_vertical_accuracy_m = c.vertical_accuracy_m,
  pose_attitude_accuracy_deg = c.attitude_accuracy_deg,
  pose_vertical_datum = c.vertical_datum,
  pose_transform_version = c.transform_version,
  pose_spatial_quality = c.spatial_quality
FROM poses c WHERE p.id = c.observation_id AND p.project_id = c.project_id;

DO $$ BEGIN IF (SELECT count(*) FROM poses) <> (SELECT count(*) FROM observations WHERE pose_spatial_quality IS NOT NULL) THEN RAISE EXCEPTION 'poses migration lost rows'; END IF; END $$;

ALTER TABLE observations ADD CONSTRAINT observations_pose_shape_check CHECK ((pose_device_id IS NULL AND pose_captured_at IS NULL AND pose_standard_position IS NULL AND pose_original_position IS NULL AND pose_orientation_x IS NULL AND pose_orientation_y IS NULL AND pose_orientation_z IS NULL AND pose_orientation_w IS NULL AND pose_velocity_x IS NULL AND pose_velocity_y IS NULL AND pose_velocity_z IS NULL AND pose_horizontal_accuracy_m IS NULL AND pose_vertical_accuracy_m IS NULL AND pose_attitude_accuracy_deg IS NULL AND pose_vertical_datum IS NULL AND pose_transform_version IS NULL AND pose_spatial_quality IS NULL) OR (pose_device_id IS NOT NULL AND pose_captured_at IS NOT NULL AND pose_spatial_quality IS NOT NULL AND pose_spatial_quality IN ('usable','degraded','unusable') AND pose_horizontal_accuracy_m >= 0 AND pose_vertical_accuracy_m >= 0 AND pose_attitude_accuracy_deg >= 0));

ALTER TABLE observations ADD CONSTRAINT observations_pose_device_id_fkey FOREIGN KEY (pose_device_id,project_id) REFERENCES devices(id,project_id) ON DELETE SET NULL (pose_device_id);

CREATE FUNCTION clear_observations_pose_extension() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.pose_spatial_quality IS NOT NULL AND (NEW.pose_device_id IS NULL) THEN
    NEW.pose_device_id := NULL;
    NEW.pose_captured_at := NULL;
    NEW.pose_standard_position := NULL;
    NEW.pose_original_position := NULL;
    NEW.pose_orientation_x := NULL;
    NEW.pose_orientation_y := NULL;
    NEW.pose_orientation_z := NULL;
    NEW.pose_orientation_w := NULL;
    NEW.pose_velocity_x := NULL;
    NEW.pose_velocity_y := NULL;
    NEW.pose_velocity_z := NULL;
    NEW.pose_horizontal_accuracy_m := NULL;
    NEW.pose_vertical_accuracy_m := NULL;
    NEW.pose_attitude_accuracy_deg := NULL;
    NEW.pose_vertical_datum := NULL;
    NEW.pose_transform_version := NULL;
    NEW.pose_spatial_quality := NULL;
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER clear_observations_pose_extension BEFORE UPDATE ON observations FOR EACH ROW EXECUTE FUNCTION clear_observations_pose_extension();

DROP TABLE poses;

CREATE VIEW poses AS SELECT id AS observation_id,
  project_id,
  pose_device_id AS device_id,
  pose_captured_at AS captured_at,
  pose_standard_position AS standard_position,
  pose_original_position AS original_position,
  pose_orientation_x AS orientation_x,
  pose_orientation_y AS orientation_y,
  pose_orientation_z AS orientation_z,
  pose_orientation_w AS orientation_w,
  pose_velocity_x AS velocity_x,
  pose_velocity_y AS velocity_y,
  pose_velocity_z AS velocity_z,
  pose_horizontal_accuracy_m AS horizontal_accuracy_m,
  pose_vertical_accuracy_m AS vertical_accuracy_m,
  pose_attitude_accuracy_deg AS attitude_accuracy_deg,
  pose_vertical_datum AS vertical_datum,
  pose_transform_version AS transform_version,
  pose_spatial_quality AS spatial_quality
FROM observations WHERE pose_spatial_quality IS NOT NULL;

CREATE FUNCTION write_poses_extension() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    UPDATE observations SET pose_device_id = NULL, pose_captured_at = NULL, pose_standard_position = NULL, pose_original_position = NULL, pose_orientation_x = NULL, pose_orientation_y = NULL, pose_orientation_z = NULL, pose_orientation_w = NULL, pose_velocity_x = NULL, pose_velocity_y = NULL, pose_velocity_z = NULL, pose_horizontal_accuracy_m = NULL, pose_vertical_accuracy_m = NULL, pose_attitude_accuracy_deg = NULL, pose_vertical_datum = NULL, pose_transform_version = NULL, pose_spatial_quality = NULL WHERE id = OLD.observation_id AND project_id = OLD.project_id;
    RETURN OLD;
  END IF;
  IF TG_OP = 'UPDATE' AND (NEW.observation_id IS DISTINCT FROM OLD.observation_id OR NEW.project_id IS DISTINCT FROM OLD.project_id) THEN
    RAISE EXCEPTION 'extension identity cannot change' USING ERRCODE = '23514';
  END IF;
  IF TG_OP = 'INSERT' THEN
    NEW.spatial_quality := COALESCE(NEW.spatial_quality,'usable');
    IF NOT EXISTS (SELECT 1 FROM observations WHERE id = NEW.observation_id AND project_id = NEW.project_id) THEN
      RAISE EXCEPTION 'extension parent does not exist in scope' USING ERRCODE = '23503';
    END IF;
    UPDATE observations SET pose_device_id = NEW.device_id,
    pose_captured_at = NEW.captured_at,
    pose_standard_position = NEW.standard_position,
    pose_original_position = NEW.original_position,
    pose_orientation_x = NEW.orientation_x,
    pose_orientation_y = NEW.orientation_y,
    pose_orientation_z = NEW.orientation_z,
    pose_orientation_w = NEW.orientation_w,
    pose_velocity_x = NEW.velocity_x,
    pose_velocity_y = NEW.velocity_y,
    pose_velocity_z = NEW.velocity_z,
    pose_horizontal_accuracy_m = NEW.horizontal_accuracy_m,
    pose_vertical_accuracy_m = NEW.vertical_accuracy_m,
    pose_attitude_accuracy_deg = NEW.attitude_accuracy_deg,
    pose_vertical_datum = NEW.vertical_datum,
    pose_transform_version = NEW.transform_version,
    pose_spatial_quality = NEW.spatial_quality WHERE id = NEW.observation_id AND project_id = NEW.project_id AND pose_spatial_quality IS NULL;
    IF NOT FOUND THEN
      RAISE EXCEPTION 'extension already exists' USING ERRCODE = '23505';
    END IF;
  ELSE
    UPDATE observations SET pose_device_id = NEW.device_id,
    pose_captured_at = NEW.captured_at,
    pose_standard_position = NEW.standard_position,
    pose_original_position = NEW.original_position,
    pose_orientation_x = NEW.orientation_x,
    pose_orientation_y = NEW.orientation_y,
    pose_orientation_z = NEW.orientation_z,
    pose_orientation_w = NEW.orientation_w,
    pose_velocity_x = NEW.velocity_x,
    pose_velocity_y = NEW.velocity_y,
    pose_velocity_z = NEW.velocity_z,
    pose_horizontal_accuracy_m = NEW.horizontal_accuracy_m,
    pose_vertical_accuracy_m = NEW.vertical_accuracy_m,
    pose_attitude_accuracy_deg = NEW.attitude_accuracy_deg,
    pose_vertical_datum = NEW.vertical_datum,
    pose_transform_version = NEW.transform_version,
    pose_spatial_quality = NEW.spatial_quality WHERE id = OLD.observation_id AND project_id = OLD.project_id;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER write_poses_extension INSTEAD OF INSERT OR UPDATE OR DELETE ON poses
FOR EACH ROW EXECUTE FUNCTION write_poses_extension();

CREATE UNIQUE INDEX device_commands_protocol_id_key ON device_commands(protocol_correlation_id) WHERE protocol_status IS NOT NULL;

CREATE UNIQUE INDEX device_commands_protocol_transaction_key ON device_commands(protocol_adapter_id,protocol_transaction_id) WHERE protocol_status IS NOT NULL;

CREATE UNIQUE INDEX device_commands_protocol_business_key ON device_commands(protocol_adapter_id,protocol_business_id,protocol_method) WHERE protocol_status IS NOT NULL;

CREATE INDEX device_commands_protocol_reply_idx ON device_commands(protocol_adapter_id,protocol_transaction_id,protocol_business_id,protocol_method,protocol_status) WHERE protocol_status IS NOT NULL;

CREATE UNIQUE INDEX assets_remote_reference_key ON assets(project_id,remote_connector_id,remote_access_kind,remote_reference_digest) WHERE remote_access_kind IS NOT NULL;

CREATE INDEX assets_remote_resource_idx ON assets(project_id,remote_connector_id,remote_resource_id) WHERE remote_access_kind IS NOT NULL;

CREATE INDEX observations_pose_device_time_idx ON observations(pose_device_id,pose_captured_at,id) WHERE pose_spatial_quality IS NOT NULL;

CREATE INDEX observations_pose_project_time_idx ON observations(project_id,pose_captured_at,id) WHERE pose_spatial_quality IS NOT NULL;

CREATE INDEX observations_pose_position_idx ON observations USING gist(pose_standard_position) WHERE pose_spatial_quality IS NOT NULL;

COMMENT ON COLUMN assets.remote_credential_envelope_json IS 'Private encrypted access locator; never include in public asset projections. Encryption AAD remains asset ID and project ID.';

COMMENT ON COLUMN observations.pose_captured_at IS 'Preserves the pose timestamp independently of the generic observation timestamp for legacy records.';
