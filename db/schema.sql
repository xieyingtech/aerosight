CREATE EXTENSION IF NOT EXISTS postgis;
--> statement-breakpoint
CREATE TABLE "agent_messages" (
	"id" serial PRIMARY KEY NOT NULL,
	"session_id" integer NOT NULL,
	"role" text NOT NULL,
	"content" text NOT NULL,
	"tool_calls_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"token_usage_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "agent_sessions" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"agent_id" integer,
	"task_run_id" integer,
	"issue_id" integer,
	"status" text DEFAULT 'open' NOT NULL,
	"started_by_user_id" integer,
	"summary" text,
	"started_at" timestamp DEFAULT now() NOT NULL,
	"ended_at" timestamp,
	"created_at" timestamp DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "agents" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"name" text NOT NULL,
	"description" text,
	"status" text DEFAULT 'disabled' NOT NULL,
	"config_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "algorithm_providers" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"name" text NOT NULL,
	"provider_type" text NOT NULL,
	"base_url" text NOT NULL,
	"secret_ref" text,
	"auth_type" text DEFAULT 'none' NOT NULL,
	"allowed_headers_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"timeout_seconds" integer DEFAULT 30 NOT NULL,
	"concurrency_limit" integer DEFAULT 1 NOT NULL,
	"rate_limit_per_minute" integer DEFAULT 60 NOT NULL,
	"status" text DEFAULT 'disabled' NOT NULL,
	"health_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "algorithm_providers_type_valid" CHECK (provider_type in ('http-json','kserve-v2','ogc-processes','ai-sdk')),
	CONSTRAINT "algorithm_providers_auth_valid" CHECK (auth_type in ('none','bearer','api-key-header','basic','signed')),
	CONSTRAINT "algorithm_providers_status_valid" CHECK (status in ('disabled','testing','active','degraded','failed')),
	CONSTRAINT "algorithm_providers_limits_valid" CHECK (timeout_seconds between 1 and 3600 and concurrency_limit > 0 and rate_limit_per_minute > 0),
	CONSTRAINT "algorithm_providers_project_name_unique" UNIQUE("project_id", "name"),
	CONSTRAINT "algorithm_providers_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "algorithm_definitions" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"provider_id" bigint NOT NULL,
	"name" text NOT NULL,
	"capability_code" text NOT NULL,
	"description" text,
	"current_published_version_id" bigint,
	"created_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "algorithm_definitions_project_name_unique" UNIQUE("project_id", "name"),
	CONSTRAINT "algorithm_definitions_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "algorithm_definition_versions" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"algorithm_definition_id" bigint NOT NULL,
	"version" integer NOT NULL,
	"status" text DEFAULT 'draft' NOT NULL,
	"execution_mode" text NOT NULL,
	"model_or_process" text NOT NULL,
	"input_requirements_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"parameters_schema_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"protocol_config_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"output_mapping_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"label_mapping_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"publish_threshold" double precision DEFAULT 0 NOT NULL,
	"created_by_user_id" integer,
	"published_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"published_at" timestamp with time zone,
	CONSTRAINT "algorithm_definition_versions_status_valid" CHECK (status in ('draft','published','retired')),
	CONSTRAINT "algorithm_definition_versions_mode_valid" CHECK (execution_mode in ('synchronous','asynchronous','callback')),
	CONSTRAINT "algorithm_definition_versions_threshold_valid" CHECK (publish_threshold between 0 and 1),
	CONSTRAINT "algorithm_definition_versions_version_valid" CHECK (version > 0),
	CONSTRAINT "algorithm_definition_versions_definition_version_unique" UNIQUE("algorithm_definition_id", "version"),
	CONSTRAINT "algorithm_definition_versions_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "algorithm_runs" (
	"id" uuid PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"algorithm_definition_version_id" bigint NOT NULL,
	"input_asset_id" integer NOT NULL,
	"task_run_id" integer,
	"device_id" integer,
	"idempotency_key" text NOT NULL,
	"status" text DEFAULT 'queued' NOT NULL,
	"parameters_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"input_snapshot_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"external_job_id" text,
	"callback_token_hash" text,
	"raw_result_object_key" text,
	"raw_result_checksum_sha256" text,
	"canonical_result_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"started_at" timestamp with time zone,
	"finished_at" timestamp with time zone,
	"error_code" text,
	"error_message" text,
	CONSTRAINT "algorithm_runs_status_valid" CHECK (status in ('queued','running','polling','waiting_callback','succeeded','failed','canceled','timed_out')),
	CONSTRAINT "algorithm_runs_checksum_valid" CHECK (raw_result_checksum_sha256 is null or raw_result_checksum_sha256 ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "algorithm_runs_project_idempotency_unique" UNIQUE("project_id", "idempotency_key"),
	CONSTRAINT "algorithm_runs_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "algorithm_run_attempts" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"algorithm_run_id" uuid NOT NULL,
	"attempt" integer NOT NULL,
	"status" text NOT NULL,
	"request_hash" text NOT NULL,
	"response_status" integer,
	"external_job_id" text,
	"duration_ms" integer,
	"error_category" text,
	"billing_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"started_at" timestamp with time zone DEFAULT now() NOT NULL,
	"finished_at" timestamp with time zone,
	CONSTRAINT "algorithm_run_attempts_status_valid" CHECK (status in ('running','succeeded','failed','timed_out','rate_limited')),
	CONSTRAINT "algorithm_run_attempts_attempt_valid" CHECK (attempt > 0 and (duration_ms is null or duration_ms >= 0)),
	CONSTRAINT "algorithm_run_attempts_run_attempt_unique" UNIQUE("algorithm_run_id", "attempt")
);
--> statement-breakpoint
CREATE TABLE "approval_requests" (
	"id" uuid PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"resource_type" text NOT NULL,
	"resource_id" text NOT NULL,
	"action" text NOT NULL,
	"requested_by_user_id" integer NOT NULL,
	"status" text DEFAULT 'pending' NOT NULL,
	"required_approvals" integer DEFAULT 1 NOT NULL,
	"require_separation" boolean DEFAULT true NOT NULL,
	"expires_at" timestamp with time zone NOT NULL,
	"context_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"decided_at" timestamp with time zone,
	CONSTRAINT "approval_requests_status_valid" CHECK (status in ('pending', 'approved', 'rejected', 'expired')),
	CONSTRAINT "approval_requests_required_valid" CHECK (required_approvals > 0),
	CONSTRAINT "approval_requests_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "approvals" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"approval_request_id" uuid NOT NULL,
	"approver_user_id" integer NOT NULL,
	"decision" text NOT NULL,
	"reason" text NOT NULL,
	"decided_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "approvals_decision_valid" CHECK (decision in ('approved', 'rejected')),
	CONSTRAINT "approvals_request_approver_unique" UNIQUE("approval_request_id", "approver_user_id")
);
--> statement-breakpoint
CREATE TABLE "assets" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"device_id" integer,
	"task_run_id" integer,
	"issue_id" integer,
	"kind" text NOT NULL,
	"mime_type" text,
	"storage_key" text NOT NULL,
	"logical_key" text NOT NULL,
	"version" integer DEFAULT 1 NOT NULL,
	"status" text DEFAULT 'available' NOT NULL,
	"object_version" text,
	"size_bytes" bigint,
	"checksum" text,
	"checksum_sha256" text,
	"captured_at" timestamp,
	"metadata_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"available_at" timestamp with time zone,
	"failed_at" timestamp with time zone,
	"failure_code" text,
	"retention_hold_until" timestamp with time zone,
	"legal_hold" boolean DEFAULT false NOT NULL,
	"retention_reason" text,
	"deleted_at" timestamp with time zone,
	"supersedes_asset_id" integer,
	CONSTRAINT "assets_status_valid" CHECK (status in ('pending', 'available', 'failed', 'deleted')),
	CONSTRAINT "assets_version_positive" CHECK (version > 0),
	CONSTRAINT "assets_checksum_sha256_valid" CHECK (checksum_sha256 is null or checksum_sha256 ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "assets_id_project_unique" UNIQUE("id", "project_id"),
	CONSTRAINT "assets_project_logical_version_unique" UNIQUE("project_id", "logical_key", "version")
);
--> statement-breakpoint
CREATE TABLE "asset_upload_intents" (
	"id" uuid PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"actor_user_id" integer,
	"logical_key" text NOT NULL,
	"object_key" text NOT NULL,
	"file_name" text NOT NULL,
	"kind" text NOT NULL,
	"mime_type" text NOT NULL,
	"expected_size_bytes" bigint NOT NULL,
	"expected_checksum_sha256" text NOT NULL,
	"device_id" integer,
	"task_run_id" integer,
	"issue_id" integer,
	"status" text DEFAULT 'pending' NOT NULL,
	"asset_id" integer,
	"failure_code" text,
	"expires_at" timestamp with time zone NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"completed_at" timestamp with time zone,
	CONSTRAINT "asset_upload_intents_status_valid" CHECK (status in ('pending', 'completed', 'failed', 'expired')),
	CONSTRAINT "asset_upload_intents_size_valid" CHECK (expected_size_bytes >= 0),
	CONSTRAINT "asset_upload_intents_checksum_valid" CHECK (expected_checksum_sha256 ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "asset_upload_intents_project_object_unique" UNIQUE("project_id", "object_key")
);
--> statement-breakpoint
CREATE TABLE "asset_derivatives" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"source_asset_id" integer NOT NULL,
	"derived_asset_id" integer NOT NULL,
	"derivative_type" text NOT NULL,
	"generator" text NOT NULL,
	"generator_version" text,
	"parameters_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "asset_derivatives_not_self" CHECK (source_asset_id <> derived_asset_id),
	CONSTRAINT "asset_derivatives_unique" UNIQUE("source_asset_id", "derived_asset_id", "derivative_type")
);
--> statement-breakpoint
CREATE TABLE "evidence_links" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"target_type" text NOT NULL,
	"target_id" text NOT NULL,
	"asset_id" integer NOT NULL,
	"asset_version" integer NOT NULL,
	"asset_checksum_sha256" text NOT NULL,
	"start_offset_ms" bigint,
	"end_offset_ms" bigint,
	"is_published" boolean DEFAULT false NOT NULL,
	"created_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "evidence_links_target_type_valid" CHECK (target_type in ('detection', 'track', 'event', 'report', 'issue', 'task_run')),
	CONSTRAINT "evidence_links_offsets_valid" CHECK ((start_offset_ms is null and end_offset_ms is null) or (start_offset_ms is not null and start_offset_ms >= 0 and end_offset_ms is not null and end_offset_ms > start_offset_ms)),
	CONSTRAINT "evidence_links_version_positive" CHECK (asset_version > 0),
	CONSTRAINT "evidence_links_checksum_valid" CHECK (asset_checksum_sha256 ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "evidence_links_unique" UNIQUE("project_id", "target_type", "target_id", "asset_id", "start_offset_ms", "end_offset_ms")
);
--> statement-breakpoint
CREATE TABLE "live_streams" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"device_id" integer NOT NULL,
	"task_run_id" integer,
	"adapter_id" bigint,
	"stream_key" text NOT NULL,
	"source_type" text NOT NULL,
	"status" text DEFAULT 'starting' NOT NULL,
	"playback_ref" text,
	"playback_locator_expires_at" timestamp with time zone,
	"status_reason" text,
	"started_by_user_id" integer,
	"started_at" timestamp with time zone DEFAULT now() NOT NULL,
	"last_active_at" timestamp with time zone,
	"ended_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "live_streams_status_valid" CHECK (status in ('starting', 'live', 'degraded', 'failed', 'stopping', 'stopped')),
	CONSTRAINT "live_streams_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE OR REPLACE FUNCTION protect_published_evidence_link()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF OLD.is_published THEN
		RAISE EXCEPTION 'published evidence links are immutable' USING ERRCODE = '55000';
	END IF;
	RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;
--> statement-breakpoint
CREATE TRIGGER evidence_links_published_immutable
BEFORE UPDATE OR DELETE ON evidence_links
FOR EACH ROW EXECUTE FUNCTION protect_published_evidence_link();
--> statement-breakpoint
CREATE TABLE "audit_events" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"request_id" text NOT NULL,
	"idempotency_key" text,
	"actor_user_id" integer,
	"actor_agent_id" integer,
	"action" text NOT NULL,
	"resource_type" text NOT NULL,
	"resource_id" text,
	"input_hash" text NOT NULL,
	"policy_result_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"result_hash" text,
	"status" text DEFAULT 'accepted' NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"completed_at" timestamp with time zone,
	CONSTRAINT "audit_events_actor_present" CHECK (actor_user_id is not null or actor_agent_id is not null),
	CONSTRAINT "audit_events_status_valid" CHECK (status in ('accepted', 'completed'))
);
--> statement-breakpoint
CREATE TABLE "device_adapters" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"name" text NOT NULL,
	"adapter_type" text NOT NULL,
	"vendor" text,
	"protocol_version" text DEFAULT '1' NOT NULL,
	"status" text DEFAULT 'disabled' NOT NULL,
	"secret_ref" text,
	"config_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"capabilities_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"last_health_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"last_checked_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "device_adapters_project_name_unique" UNIQUE("project_id", "name"),
	CONSTRAINT "device_adapters_id_project_unique" UNIQUE("id", "project_id"),
	CONSTRAINT "device_adapters_status_valid" CHECK (status in ('disabled', 'connecting', 'connected', 'degraded', 'failed'))
);
--> statement-breakpoint
CREATE TABLE "device_capabilities" (
	"id" serial PRIMARY KEY NOT NULL,
	"device_id" integer NOT NULL,
	"project_id" integer NOT NULL,
	"capability_code" text NOT NULL,
	"version" text,
	"version_number" integer DEFAULT 1 NOT NULL,
	"declared_by_adapter_id" bigint,
	"params_schema_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"constraints_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "devices" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"name" text NOT NULL,
	"type" text NOT NULL,
	"adapter_id" bigint,
	"device_model" text,
	"firmware_version" text,
	"status" text DEFAULT 'offline' NOT NULL,
	"status_reason" text,
	"last_seen_at" timestamp,
	"config_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"metadata_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "devices_id_project_unique" UNIQUE("id", "project_id"),
	CONSTRAINT "devices_connectivity_status_valid" CHECK (status in ('online', 'degraded', 'offline', 'unknown'))
);
--> statement-breakpoint
CREATE TABLE "device_external_identities" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"adapter_id" bigint NOT NULL,
	"device_id" integer,
	"external_device_id" text NOT NULL,
	"external_device_type" text,
	"identity_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"first_seen_at" timestamp with time zone DEFAULT now() NOT NULL,
	"last_seen_at" timestamp with time zone DEFAULT now() NOT NULL,
	"bound_at" timestamp with time zone,
	CONSTRAINT "device_external_identities_adapter_external_unique" UNIQUE("adapter_id", "external_device_id"),
	CONSTRAINT "device_external_identities_device_adapter_unique" UNIQUE("device_id", "adapter_id")
);
--> statement-breakpoint
CREATE TABLE "device_connections" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"adapter_id" bigint NOT NULL,
	"device_id" integer,
	"session_key" text NOT NULL,
	"status" text DEFAULT 'unknown' NOT NULL,
	"link_quality" double precision,
	"status_reason" text,
	"opened_at" timestamp with time zone DEFAULT now() NOT NULL,
	"last_heartbeat_at" timestamp with time zone,
	"heartbeat_interval_seconds" integer DEFAULT 30 NOT NULL,
	"status_projected_at" timestamp with time zone,
	"closed_at" timestamp with time zone,
	"metadata_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT "device_connections_session_unique" UNIQUE("adapter_id", "session_key"),
	CONSTRAINT "device_connections_status_valid" CHECK (status in ('online', 'degraded', 'offline', 'unknown')),
	CONSTRAINT "device_connections_heartbeat_interval_valid" CHECK (heartbeat_interval_seconds between 5 and 3600)
);
--> statement-breakpoint
CREATE TABLE "device_telemetry" (
	"id" bigserial NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"adapter_id" bigint NOT NULL,
	"device_id" integer NOT NULL,
	"event_id" text NOT NULL,
	"telemetry_type" text NOT NULL,
	"sequence_number" bigint,
	"captured_at" timestamp with time zone NOT NULL,
	"received_at" timestamp with time zone DEFAULT now() NOT NULL,
	"payload_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"quality_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT "device_telemetry_pk" PRIMARY KEY("id", "captured_at")
) PARTITION BY RANGE (captured_at);
--> statement-breakpoint
CREATE TABLE "device_telemetry_default" PARTITION OF "device_telemetry" DEFAULT;
--> statement-breakpoint
CREATE TABLE "telemetry_event_dedup" (
	"adapter_id" bigint NOT NULL,
	"event_id" text NOT NULL,
	"project_id" integer NOT NULL,
	"captured_at" timestamp with time zone NOT NULL,
	"received_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "telemetry_event_dedup_pk" PRIMARY KEY("adapter_id", "event_id")
);
--> statement-breakpoint
CREATE TABLE "device_latest_telemetry" (
	"device_id" integer PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"adapter_id" bigint NOT NULL,
	"event_id" text NOT NULL,
	"telemetry_type" text NOT NULL,
	"sequence_number" bigint,
	"captured_at" timestamp with time zone NOT NULL,
	"received_at" timestamp with time zone NOT NULL,
	"payload_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"quality_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "coordinate_references" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"code" text NOT NULL,
	"name" text NOT NULL,
	"authority" text,
	"definition" text,
	"vertical_datum" text,
	"transform_version" text DEFAULT '1' NOT NULL,
	"is_project_standard" boolean DEFAULT false NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "coordinate_references_project_code_version_unique" UNIQUE("project_id", "code", "transform_version"),
	CONSTRAINT "coordinate_references_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "sensor_calibrations" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"device_id" integer NOT NULL,
	"sensor_key" text NOT NULL,
	"version" integer NOT NULL,
	"intrinsic_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"extrinsic_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"quality_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"valid_from" timestamp with time zone NOT NULL,
	"valid_until" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "sensor_calibrations_device_sensor_version_unique" UNIQUE("device_id", "sensor_key", "version"),
	CONSTRAINT "sensor_calibrations_id_project_unique" UNIQUE("id", "project_id"),
	CONSTRAINT "sensor_calibrations_valid_range" CHECK (valid_until is null or valid_until > valid_from)
);
--> statement-breakpoint
CREATE TABLE "observations" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"adapter_id" bigint NOT NULL,
	"device_id" integer NOT NULL,
	"calibration_id" bigint,
	"observation_type" text NOT NULL,
	"source_event_id" text NOT NULL,
	"captured_at" timestamp with time zone NOT NULL,
	"received_at" timestamp with time zone NOT NULL,
	"time_quality" text DEFAULT 'trusted' NOT NULL,
	"original_crs_id" bigint,
	"original_geometry" geometry(GeometryZ),
	"standard_geometry" geometry(GeometryZ, 4326),
	"properties_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"quality_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"validity" text DEFAULT 'valid' NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "observations_source_unique" UNIQUE("adapter_id", "source_event_id"),
	CONSTRAINT "observations_id_project_unique" UNIQUE("id", "project_id"),
	CONSTRAINT "observations_time_quality_valid" CHECK (time_quality in ('trusted', 'uncertain', 'invalid')),
	CONSTRAINT "observations_validity_valid" CHECK (validity in ('valid', 'degraded', 'late', 'invalid'))
);
--> statement-breakpoint
CREATE TABLE "poses" (
	"observation_id" bigint PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"device_id" integer NOT NULL,
	"captured_at" timestamp with time zone NOT NULL,
	"standard_position" geometry(PointZ, 4326),
	"original_position" geometry(PointZ),
	"orientation_x" double precision,
	"orientation_y" double precision,
	"orientation_z" double precision,
	"orientation_w" double precision,
	"velocity_x" double precision,
	"velocity_y" double precision,
	"velocity_z" double precision,
	"horizontal_accuracy_m" double precision,
	"vertical_accuracy_m" double precision,
	"attitude_accuracy_deg" double precision,
	"vertical_datum" text,
	"transform_version" text,
	"spatial_quality" text DEFAULT 'usable' NOT NULL,
	CONSTRAINT "poses_spatial_quality_valid" CHECK (spatial_quality in ('usable', 'degraded', 'unusable')),
	CONSTRAINT "poses_accuracy_nonnegative" CHECK ((horizontal_accuracy_m is null or horizontal_accuracy_m >= 0) and (vertical_accuracy_m is null or vertical_accuracy_m >= 0) and (attitude_accuracy_deg is null or attitude_accuracy_deg >= 0))
);
--> statement-breakpoint
CREATE TABLE "issue_events" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"issue_id" integer NOT NULL,
	"event_type" text NOT NULL,
	"body" text,
	"metadata_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"actor_user_id" integer,
	"actor_agent_id" integer,
	"created_at" timestamp DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "idempotency_records" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"actor_key" text NOT NULL,
	"operation" text NOT NULL,
	"idempotency_key" text NOT NULL,
	"request_hash" text NOT NULL,
	"status" text DEFAULT 'processing' NOT NULL,
	"response_json" jsonb,
	"error_code" text,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"completed_at" timestamp with time zone,
	"expires_at" timestamp with time zone DEFAULT (now() + interval '24 hours') NOT NULL,
	CONSTRAINT "idempotency_records_scope_unique" UNIQUE("project_id", "actor_key", "operation", "idempotency_key"),
	CONSTRAINT "idempotency_records_status_valid" CHECK (status in ('processing', 'completed', 'failed'))
);
--> statement-breakpoint
CREATE TABLE "issue_links" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"issue_id" integer NOT NULL,
	"link_type" text NOT NULL,
	"target_id" integer NOT NULL,
	"created_by_user_id" integer,
	"created_at" timestamp DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "issues" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"number" integer NOT NULL,
	"title" text NOT NULL,
	"description" text,
	"source_type" text NOT NULL,
	"source_id" integer,
	"status" text DEFAULT 'open' NOT NULL,
	"priority" text DEFAULT 'medium' NOT NULL,
	"task_run_id" integer,
	"opened_by_user_id" integer,
	"assignee_user_id" integer,
	"closed_at" timestamp,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "issues_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "outbox_events" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"event_id" text NOT NULL,
	"event_type" text NOT NULL,
	"aggregate_type" text,
	"aggregate_id" text,
	"payload_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"status" text DEFAULT 'pending' NOT NULL,
	"attempts" integer DEFAULT 0 NOT NULL,
	"max_attempts" integer DEFAULT 8 NOT NULL,
	"available_at" timestamp with time zone DEFAULT now() NOT NULL,
	"locked_by" text,
	"locked_until" timestamp with time zone,
	"last_error" text,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"completed_at" timestamp with time zone,
	CONSTRAINT "outbox_events_event_unique" UNIQUE("event_id"),
	CONSTRAINT "outbox_events_status_valid" CHECK (status in ('pending', 'processing', 'completed', 'dead')),
	CONSTRAINT "outbox_events_attempts_valid" CHECK (attempts >= 0 and max_attempts > 0)
);
--> statement-breakpoint
CREATE TABLE "outbox_consumptions" (
	"consumer_name" text NOT NULL,
	"event_id" text NOT NULL,
	"consumed_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "outbox_consumptions_pk" PRIMARY KEY("consumer_name", "event_id")
);
--> statement-breakpoint
CREATE TABLE "projects" (
	"id" serial PRIMARY KEY NOT NULL,
	"team_id" integer NOT NULL,
	"name" text NOT NULL,
	"description" text,
	"created_by_user_id" integer,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL,
	"current_safety_policy_version_id" bigint,
	CONSTRAINT "projects_id_team_unique" UNIQUE("id", "team_id")
);
--> statement-breakpoint
CREATE TABLE "project_feature_flags" (
	"project_id" integer PRIMARY KEY NOT NULL,
	"device_commands_enabled" boolean DEFAULT false NOT NULL,
	"operations_overview_enabled" boolean DEFAULT false NOT NULL,
	"object_storage_enabled" boolean DEFAULT false NOT NULL,
	"external_algorithms_enabled" boolean DEFAULT false NOT NULL,
	"automatic_ai_enabled" boolean DEFAULT false NOT NULL,
	"dependency_health_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"updated_by_user_id" integer,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "project_permissions" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"user_id" integer NOT NULL,
	"permission" text NOT NULL,
	"granted_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "project_permissions_unique" UNIQUE("project_id", "user_id", "permission")
);
--> statement-breakpoint
CREATE TABLE "project_events" (
	"cursor" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"event_id" text NOT NULL,
	"event_type" text NOT NULL,
	"payload_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"occurred_at" timestamp with time zone DEFAULT now() NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "project_events_event_unique" UNIQUE("event_id")
);
--> statement-breakpoint
CREATE TABLE "safety_policy_versions" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"version" integer NOT NULL,
	"status" text DEFAULT 'draft' NOT NULL,
	"project_boundary" geometry(Polygon,4326),
	"restricted_areas" geometry(MultiPolygon,4326),
	"max_altitude_meters" double precision NOT NULL,
	"max_speed_meters_per_second" double precision NOT NULL,
	"minimum_battery_percent" double precision NOT NULL,
	"allowed_windows_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"required_compliance_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"optional_compliance_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"exemptions_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"created_by_user_id" integer,
	"published_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"published_at" timestamp with time zone,
	CONSTRAINT "safety_policy_versions_status_valid" CHECK (status in ('draft', 'published')),
	CONSTRAINT "safety_policy_versions_limits_valid" CHECK (max_altitude_meters > 0 and max_speed_meters_per_second > 0 and minimum_battery_percent between 0 and 100),
	CONSTRAINT "safety_policy_versions_project_version_unique" UNIQUE("project_id", "version"),
	CONSTRAINT "safety_policy_versions_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "task_runs" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"task_id" integer NOT NULL,
	"task_version_id" bigint,
	"selected_device_id" integer,
	"safety_policy_version_id" bigint,
	"approval_request_id" uuid,
	"trigger_source" text NOT NULL,
	"status" text DEFAULT 'queued' NOT NULL,
	"state_version" integer DEFAULT 0 NOT NULL,
	"current_step_position" integer,
	"input_snapshot_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"preflight_snapshot_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"output_snapshot_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"state_reason" text,
	"error_message" text,
	"started_at" timestamp,
	"finished_at" timestamp,
	"created_by_user_id" integer,
	"created_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "task_runs_id_project_unique" UNIQUE("id", "project_id"),
	CONSTRAINT "task_runs_status_valid" CHECK (status in ('queued','blocked','ready','dispatching','running','paused','succeeded','failed','canceling','canceled')),
	CONSTRAINT "task_runs_state_version_valid" CHECK (state_version >= 0),
	CONSTRAINT "task_runs_current_step_valid" CHECK (current_step_position is null or current_step_position > 0)
);
--> statement-breakpoint
CREATE TABLE "task_run_steps" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"task_run_id" integer NOT NULL,
	"task_step_id" bigint NOT NULL,
	"position" integer NOT NULL,
	"status" text DEFAULT 'pending' NOT NULL,
	"attempt_count" integer DEFAULT 0 NOT NULL,
	"result_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"started_at" timestamp with time zone,
	"finished_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "task_run_steps_status_valid" CHECK (status in ('pending','dispatching','running','succeeded','failed','skipped','paused')),
	CONSTRAINT "task_run_steps_position_valid" CHECK (position > 0 and attempt_count >= 0),
	CONSTRAINT "task_run_steps_run_position_unique" UNIQUE("task_run_id", "position"),
	CONSTRAINT "task_run_steps_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "tasks" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"name" text NOT NULL,
	"description" text,
	"trigger_type" text NOT NULL,
	"status" text DEFAULT 'active' NOT NULL,
	"required_capability_code" text,
	"target_selector_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"schedule" text,
	"event_rule_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"script" text NOT NULL,
	"created_by_user_id" integer,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL,
	"current_published_version_id" bigint,
	CONSTRAINT "tasks_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "task_versions" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"task_id" integer NOT NULL,
	"version" integer NOT NULL,
	"status" text DEFAULT 'draft' NOT NULL,
	"definition_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"script" text NOT NULL,
	"created_by_user_id" integer,
	"published_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"published_at" timestamp with time zone,
	CONSTRAINT "task_versions_status_valid" CHECK (status in ('draft', 'published', 'retired')),
	CONSTRAINT "task_versions_version_positive" CHECK (version > 0),
	CONSTRAINT "task_versions_task_version_unique" UNIQUE("task_id", "version"),
	CONSTRAINT "task_versions_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "task_steps" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"task_version_id" bigint NOT NULL,
	"position" integer NOT NULL,
	"step_key" text NOT NULL,
	"name" text NOT NULL,
	"capability_code" text,
	"action" text NOT NULL,
	"parameters_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"failure_policy_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"media_requirements_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "task_steps_position_positive" CHECK (position > 0),
	CONSTRAINT "task_steps_version_position_unique" UNIQUE("task_version_id", "position"),
	CONSTRAINT "task_steps_version_key_unique" UNIQUE("task_version_id", "step_key"),
	CONSTRAINT "task_steps_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "device_commands" (
	"id" uuid PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"task_run_id" integer NOT NULL,
	"task_run_step_id" bigint,
	"device_id" integer NOT NULL,
	"command_key" text NOT NULL,
	"idempotency_key" text NOT NULL,
	"capability_code" text NOT NULL,
	"parameters_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"safety_context_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"status" text DEFAULT 'pending' NOT NULL,
	"priority" integer DEFAULT 0 NOT NULL,
	"deadline_at" timestamp with time zone NOT NULL,
	"requested_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"completed_at" timestamp with time zone,
	"result_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT "device_commands_status_valid" CHECK (status in ('pending','dispatchable','sent','acknowledged','nacked','timed_out','canceled','unknown')),
	CONSTRAINT "device_commands_priority_valid" CHECK (priority between 0 and 100),
	CONSTRAINT "device_commands_device_idempotency_unique" UNIQUE("device_id", "idempotency_key"),
	CONSTRAINT "device_commands_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "command_attempts" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"command_id" uuid NOT NULL,
	"adapter_id" bigint NOT NULL,
	"attempt" integer NOT NULL,
	"status" text NOT NULL,
	"sent_at" timestamp with time zone DEFAULT now() NOT NULL,
	"acknowledged_at" timestamp with time zone,
	"error_code" text,
	"result_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT "command_attempts_status_valid" CHECK (status in ('sent','acknowledged','nacked','timed_out','transport_error')),
	CONSTRAINT "command_attempts_attempt_valid" CHECK (attempt > 0),
	CONSTRAINT "command_attempts_command_attempt_unique" UNIQUE("command_id", "attempt")
);
--> statement-breakpoint
CREATE TABLE "team_members" (
	"id" serial PRIMARY KEY NOT NULL,
	"team_id" integer NOT NULL,
	"user_id" integer NOT NULL,
	"role" text DEFAULT 'member' NOT NULL,
	"created_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "team_members_team_user_unique" UNIQUE("team_id", "user_id")
);
--> statement-breakpoint
CREATE TABLE "teams" (
	"id" serial PRIMARY KEY NOT NULL,
	"name" text NOT NULL,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "users" (
	"id" serial PRIMARY KEY NOT NULL,
	"name" text NOT NULL,
	"email" text,
	"phone" text,
	"password" text,
	"role" text DEFAULT 'user' NOT NULL,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "users_email_unique" UNIQUE("email"),
	CONSTRAINT "users_phone_unique" UNIQUE("phone")
);
--> statement-breakpoint
CREATE OR REPLACE FUNCTION protect_published_task_version()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF OLD.status IN ('published', 'retired') THEN
		RAISE EXCEPTION 'published task versions are immutable' USING ERRCODE = '55000';
	END IF;
	RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;
--> statement-breakpoint
CREATE TRIGGER task_versions_published_immutable
BEFORE UPDATE OR DELETE ON task_versions
FOR EACH ROW EXECUTE FUNCTION protect_published_task_version();
--> statement-breakpoint
CREATE OR REPLACE FUNCTION protect_published_task_step()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF EXISTS (SELECT 1 FROM task_versions WHERE id = OLD.task_version_id AND status IN ('published', 'retired')) THEN
		RAISE EXCEPTION 'published task steps are immutable' USING ERRCODE = '55000';
	END IF;
	RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;
--> statement-breakpoint
CREATE TRIGGER task_steps_published_immutable
BEFORE UPDATE OR DELETE ON task_steps
FOR EACH ROW EXECUTE FUNCTION protect_published_task_step();
--> statement-breakpoint
CREATE OR REPLACE FUNCTION protect_published_safety_policy_version()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF OLD.status = 'published' THEN
		RAISE EXCEPTION 'published safety policy versions are immutable' USING ERRCODE = '55000';
	END IF;
	RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;
--> statement-breakpoint
CREATE TRIGGER safety_policy_versions_published_immutable
BEFORE UPDATE OR DELETE ON safety_policy_versions
FOR EACH ROW EXECUTE FUNCTION protect_published_safety_policy_version();
--> statement-breakpoint
CREATE OR REPLACE FUNCTION protect_published_algorithm_definition_version()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF OLD.status IN ('published','retired') THEN
		RAISE EXCEPTION 'published algorithm definition versions are immutable' USING ERRCODE = '55000';
	END IF;
	RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;
--> statement-breakpoint
CREATE TRIGGER algorithm_definition_versions_published_immutable
BEFORE UPDATE OR DELETE ON algorithm_definition_versions
FOR EACH ROW EXECUTE FUNCTION protect_published_algorithm_definition_version();
--> statement-breakpoint
CREATE OR REPLACE FUNCTION validate_approval_decision()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE request approval_requests%ROWTYPE;
BEGIN
	SELECT * INTO request FROM approval_requests WHERE id = NEW.approval_request_id FOR UPDATE;
	IF request.status <> 'pending' THEN
		RAISE EXCEPTION 'approval request is not pending' USING ERRCODE = '55000';
	END IF;
	IF request.expires_at <= NEW.decided_at THEN
		RAISE EXCEPTION 'approval request expired' USING ERRCODE = '55000';
	END IF;
	IF request.require_separation AND request.requested_by_user_id = NEW.approver_user_id THEN
		RAISE EXCEPTION 'requester cannot approve own request' USING ERRCODE = '42501';
	END IF;
	RETURN NEW;
END;
$$;
--> statement-breakpoint
CREATE TRIGGER approvals_validate_decision
BEFORE INSERT ON approvals FOR EACH ROW EXECUTE FUNCTION validate_approval_decision();
--> statement-breakpoint
CREATE OR REPLACE FUNCTION project_approval_request_status()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE required_count integer;
DECLARE approved_count integer;
BEGIN
	IF NEW.decision = 'rejected' THEN
		UPDATE approval_requests SET status = 'rejected', decided_at = NEW.decided_at WHERE id = NEW.approval_request_id;
		RETURN NEW;
	END IF;
	SELECT required_approvals INTO required_count FROM approval_requests WHERE id = NEW.approval_request_id;
	SELECT count(*) INTO approved_count FROM approvals WHERE approval_request_id = NEW.approval_request_id AND decision = 'approved';
	IF approved_count >= required_count THEN
		UPDATE approval_requests SET status = 'approved', decided_at = NEW.decided_at WHERE id = NEW.approval_request_id;
	END IF;
	RETURN NEW;
END;
$$;
--> statement-breakpoint
CREATE TRIGGER approvals_project_request_status
AFTER INSERT ON approvals FOR EACH ROW EXECUTE FUNCTION project_approval_request_status();
--> statement-breakpoint
ALTER TABLE "agent_messages" ADD CONSTRAINT "agent_messages_session_id_agent_sessions_id_fk" FOREIGN KEY ("session_id") REFERENCES "public"."agent_sessions"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_agent_id_agents_id_fk" FOREIGN KEY ("agent_id") REFERENCES "public"."agents"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_task_run_id_task_runs_id_fk" FOREIGN KEY ("task_run_id") REFERENCES "public"."task_runs"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_issue_id_issues_id_fk" FOREIGN KEY ("issue_id") REFERENCES "public"."issues"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_started_by_user_id_users_id_fk" FOREIGN KEY ("started_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agents" ADD CONSTRAINT "agents_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_providers" ADD CONSTRAINT "algorithm_providers_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_providers" ADD CONSTRAINT "algorithm_providers_creator_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_definitions" ADD CONSTRAINT "algorithm_definitions_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_definitions" ADD CONSTRAINT "algorithm_definitions_provider_project_fk" FOREIGN KEY ("provider_id","project_id") REFERENCES "public"."algorithm_providers"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_definitions" ADD CONSTRAINT "algorithm_definitions_creator_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_definitions" ADD CONSTRAINT "algorithm_definitions_current_version_project_fk" FOREIGN KEY ("current_published_version_id","project_id") REFERENCES "public"."algorithm_definition_versions"("id","project_id") ON DELETE SET NULL ("current_published_version_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_definition_versions" ADD CONSTRAINT "algorithm_definition_versions_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_definition_versions" ADD CONSTRAINT "algorithm_definition_versions_definition_project_fk" FOREIGN KEY ("algorithm_definition_id","project_id") REFERENCES "public"."algorithm_definitions"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_definition_versions" ADD CONSTRAINT "algorithm_definition_versions_creator_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_definition_versions" ADD CONSTRAINT "algorithm_definition_versions_publisher_fk" FOREIGN KEY ("published_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_runs" ADD CONSTRAINT "algorithm_runs_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_runs" ADD CONSTRAINT "algorithm_runs_version_project_fk" FOREIGN KEY ("algorithm_definition_version_id","project_id") REFERENCES "public"."algorithm_definition_versions"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_runs" ADD CONSTRAINT "algorithm_runs_asset_project_fk" FOREIGN KEY ("input_asset_id","project_id") REFERENCES "public"."assets"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_runs" ADD CONSTRAINT "algorithm_runs_task_run_project_fk" FOREIGN KEY ("task_run_id","project_id") REFERENCES "public"."task_runs"("id","project_id") ON DELETE SET NULL ("task_run_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_runs" ADD CONSTRAINT "algorithm_runs_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE SET NULL ("device_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_run_attempts" ADD CONSTRAINT "algorithm_run_attempts_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_run_attempts" ADD CONSTRAINT "algorithm_run_attempts_run_project_fk" FOREIGN KEY ("algorithm_run_id","project_id") REFERENCES "public"."algorithm_runs"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "approval_requests" ADD CONSTRAINT "approval_requests_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "approval_requests" ADD CONSTRAINT "approval_requests_requester_member_fk" FOREIGN KEY ("team_id","requested_by_user_id") REFERENCES "public"."team_members"("team_id","user_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "approvals" ADD CONSTRAINT "approvals_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "approvals" ADD CONSTRAINT "approvals_request_project_fk" FOREIGN KEY ("approval_request_id","project_id") REFERENCES "public"."approval_requests"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "approvals" ADD CONSTRAINT "approvals_approver_member_fk" FOREIGN KEY ("team_id","approver_user_id") REFERENCES "public"."team_members"("team_id","user_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "assets" ADD CONSTRAINT "assets_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "assets" ADD CONSTRAINT "assets_device_id_devices_id_fk" FOREIGN KEY ("device_id") REFERENCES "public"."devices"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "assets" ADD CONSTRAINT "assets_task_run_id_task_runs_id_fk" FOREIGN KEY ("task_run_id") REFERENCES "public"."task_runs"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "assets" ADD CONSTRAINT "assets_issue_id_issues_id_fk" FOREIGN KEY ("issue_id") REFERENCES "public"."issues"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "assets" ADD CONSTRAINT "assets_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "assets" ADD CONSTRAINT "assets_supersedes_project_fk" FOREIGN KEY ("supersedes_asset_id","project_id") REFERENCES "public"."assets"("id","project_id") ON DELETE SET NULL ("supersedes_asset_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "assets" ADD CONSTRAINT "assets_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE SET NULL ("device_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "assets" ADD CONSTRAINT "assets_task_run_project_fk" FOREIGN KEY ("task_run_id","project_id") REFERENCES "public"."task_runs"("id","project_id") ON DELETE SET NULL ("task_run_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "assets" ADD CONSTRAINT "assets_issue_project_fk" FOREIGN KEY ("issue_id","project_id") REFERENCES "public"."issues"("id","project_id") ON DELETE SET NULL ("issue_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "asset_upload_intents" ADD CONSTRAINT "asset_upload_intents_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "asset_upload_intents" ADD CONSTRAINT "asset_upload_intents_actor_fk" FOREIGN KEY ("actor_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "asset_upload_intents" ADD CONSTRAINT "asset_upload_intents_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE SET NULL ("device_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "asset_upload_intents" ADD CONSTRAINT "asset_upload_intents_task_run_project_fk" FOREIGN KEY ("task_run_id","project_id") REFERENCES "public"."task_runs"("id","project_id") ON DELETE SET NULL ("task_run_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "asset_upload_intents" ADD CONSTRAINT "asset_upload_intents_issue_project_fk" FOREIGN KEY ("issue_id","project_id") REFERENCES "public"."issues"("id","project_id") ON DELETE SET NULL ("issue_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "asset_upload_intents" ADD CONSTRAINT "asset_upload_intents_asset_project_fk" FOREIGN KEY ("asset_id","project_id") REFERENCES "public"."assets"("id","project_id") ON DELETE SET NULL ("asset_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "asset_derivatives" ADD CONSTRAINT "asset_derivatives_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "asset_derivatives" ADD CONSTRAINT "asset_derivatives_source_project_fk" FOREIGN KEY ("source_asset_id","project_id") REFERENCES "public"."assets"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "asset_derivatives" ADD CONSTRAINT "asset_derivatives_derived_project_fk" FOREIGN KEY ("derived_asset_id","project_id") REFERENCES "public"."assets"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "evidence_links" ADD CONSTRAINT "evidence_links_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "evidence_links" ADD CONSTRAINT "evidence_links_asset_project_fk" FOREIGN KEY ("asset_id","project_id") REFERENCES "public"."assets"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "evidence_links" ADD CONSTRAINT "evidence_links_created_by_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "live_streams" ADD CONSTRAINT "live_streams_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "live_streams" ADD CONSTRAINT "live_streams_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "live_streams" ADD CONSTRAINT "live_streams_task_run_project_fk" FOREIGN KEY ("task_run_id","project_id") REFERENCES "public"."task_runs"("id","project_id") ON DELETE SET NULL ("task_run_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "live_streams" ADD CONSTRAINT "live_streams_adapter_project_fk" FOREIGN KEY ("adapter_id","project_id") REFERENCES "public"."device_adapters"("id","project_id") ON DELETE SET NULL ("adapter_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "live_streams" ADD CONSTRAINT "live_streams_started_by_fk" FOREIGN KEY ("started_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "audit_events" ADD CONSTRAINT "audit_events_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "audit_events" ADD CONSTRAINT "audit_events_actor_user_id_users_id_fk" FOREIGN KEY ("actor_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "audit_events" ADD CONSTRAINT "audit_events_actor_agent_id_agents_id_fk" FOREIGN KEY ("actor_agent_id") REFERENCES "public"."agents"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_adapters" ADD CONSTRAINT "device_adapters_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_capabilities" ADD CONSTRAINT "device_capabilities_device_id_devices_id_fk" FOREIGN KEY ("device_id") REFERENCES "public"."devices"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_capabilities" ADD CONSTRAINT "device_capabilities_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_capabilities" ADD CONSTRAINT "device_capabilities_adapter_project_fk" FOREIGN KEY ("declared_by_adapter_id","project_id") REFERENCES "public"."device_adapters"("id","project_id") ON DELETE SET NULL ("declared_by_adapter_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "devices" ADD CONSTRAINT "devices_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "devices" ADD CONSTRAINT "devices_adapter_project_fk" FOREIGN KEY ("adapter_id","project_id") REFERENCES "public"."device_adapters"("id","project_id") ON DELETE SET NULL ("adapter_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_external_identities" ADD CONSTRAINT "device_external_identities_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_external_identities" ADD CONSTRAINT "device_external_identities_adapter_project_fk" FOREIGN KEY ("adapter_id","project_id") REFERENCES "public"."device_adapters"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_external_identities" ADD CONSTRAINT "device_external_identities_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_connections" ADD CONSTRAINT "device_connections_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_connections" ADD CONSTRAINT "device_connections_adapter_project_fk" FOREIGN KEY ("adapter_id","project_id") REFERENCES "public"."device_adapters"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_connections" ADD CONSTRAINT "device_connections_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_commands" ADD CONSTRAINT "device_commands_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_commands" ADD CONSTRAINT "device_commands_run_project_fk" FOREIGN KEY ("task_run_id","project_id") REFERENCES "public"."task_runs"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_commands" ADD CONSTRAINT "device_commands_run_step_project_fk" FOREIGN KEY ("task_run_step_id","project_id") REFERENCES "public"."task_run_steps"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_commands" ADD CONSTRAINT "device_commands_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_commands" ADD CONSTRAINT "device_commands_requester_fk" FOREIGN KEY ("requested_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "command_attempts" ADD CONSTRAINT "command_attempts_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "command_attempts" ADD CONSTRAINT "command_attempts_command_project_fk" FOREIGN KEY ("command_id","project_id") REFERENCES "public"."device_commands"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "command_attempts" ADD CONSTRAINT "command_attempts_adapter_project_fk" FOREIGN KEY ("adapter_id","project_id") REFERENCES "public"."device_adapters"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_telemetry" ADD CONSTRAINT "device_telemetry_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_telemetry" ADD CONSTRAINT "device_telemetry_adapter_project_fk" FOREIGN KEY ("adapter_id","project_id") REFERENCES "public"."device_adapters"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_telemetry" ADD CONSTRAINT "device_telemetry_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "telemetry_event_dedup" ADD CONSTRAINT "telemetry_event_dedup_adapter_project_fk" FOREIGN KEY ("adapter_id","project_id") REFERENCES "public"."device_adapters"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_latest_telemetry" ADD CONSTRAINT "device_latest_telemetry_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "device_latest_telemetry" ADD CONSTRAINT "device_latest_telemetry_adapter_project_fk" FOREIGN KEY ("adapter_id","project_id") REFERENCES "public"."device_adapters"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "coordinate_references" ADD CONSTRAINT "coordinate_references_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "sensor_calibrations" ADD CONSTRAINT "sensor_calibrations_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "sensor_calibrations" ADD CONSTRAINT "sensor_calibrations_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "observations" ADD CONSTRAINT "observations_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "observations" ADD CONSTRAINT "observations_adapter_project_fk" FOREIGN KEY ("adapter_id","project_id") REFERENCES "public"."device_adapters"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "observations" ADD CONSTRAINT "observations_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "observations" ADD CONSTRAINT "observations_calibration_project_fk" FOREIGN KEY ("calibration_id","project_id") REFERENCES "public"."sensor_calibrations"("id","project_id") ON DELETE SET NULL ("calibration_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "observations" ADD CONSTRAINT "observations_crs_project_fk" FOREIGN KEY ("original_crs_id","project_id") REFERENCES "public"."coordinate_references"("id","project_id") ON DELETE SET NULL ("original_crs_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "poses" ADD CONSTRAINT "poses_observation_project_fk" FOREIGN KEY ("observation_id","project_id") REFERENCES "public"."observations"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "poses" ADD CONSTRAINT "poses_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_events" ADD CONSTRAINT "issue_events_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_events" ADD CONSTRAINT "issue_events_issue_id_issues_id_fk" FOREIGN KEY ("issue_id") REFERENCES "public"."issues"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_events" ADD CONSTRAINT "issue_events_actor_user_id_users_id_fk" FOREIGN KEY ("actor_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_events" ADD CONSTRAINT "issue_events_actor_agent_id_agents_id_fk" FOREIGN KEY ("actor_agent_id") REFERENCES "public"."agents"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "idempotency_records" ADD CONSTRAINT "idempotency_records_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_links" ADD CONSTRAINT "issue_links_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_links" ADD CONSTRAINT "issue_links_issue_id_issues_id_fk" FOREIGN KEY ("issue_id") REFERENCES "public"."issues"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_links" ADD CONSTRAINT "issue_links_created_by_user_id_users_id_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issues" ADD CONSTRAINT "issues_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issues" ADD CONSTRAINT "issues_task_run_id_task_runs_id_fk" FOREIGN KEY ("task_run_id") REFERENCES "public"."task_runs"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issues" ADD CONSTRAINT "issues_opened_by_user_id_users_id_fk" FOREIGN KEY ("opened_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issues" ADD CONSTRAINT "issues_assignee_user_id_users_id_fk" FOREIGN KEY ("assignee_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "outbox_events" ADD CONSTRAINT "outbox_events_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "outbox_consumptions" ADD CONSTRAINT "outbox_consumptions_event_id_fk" FOREIGN KEY ("event_id") REFERENCES "public"."outbox_events"("event_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "projects" ADD CONSTRAINT "projects_team_id_teams_id_fk" FOREIGN KEY ("team_id") REFERENCES "public"."teams"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "projects" ADD CONSTRAINT "projects_created_by_user_id_users_id_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "projects" ADD CONSTRAINT "projects_current_safety_policy_project_fk" FOREIGN KEY ("current_safety_policy_version_id","id") REFERENCES "public"."safety_policy_versions"("id","project_id") ON DELETE SET NULL ("current_safety_policy_version_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "project_feature_flags" ADD CONSTRAINT "project_feature_flags_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "project_feature_flags" ADD CONSTRAINT "project_feature_flags_updated_by_user_id_users_id_fk" FOREIGN KEY ("updated_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "project_permissions" ADD CONSTRAINT "project_permissions_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "project_permissions" ADD CONSTRAINT "project_permissions_team_member_fk" FOREIGN KEY ("team_id","user_id") REFERENCES "public"."team_members"("team_id","user_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "project_permissions" ADD CONSTRAINT "project_permissions_granted_by_user_id_users_id_fk" FOREIGN KEY ("granted_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "project_events" ADD CONSTRAINT "project_events_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "safety_policy_versions" ADD CONSTRAINT "safety_policy_versions_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "safety_policy_versions" ADD CONSTRAINT "safety_policy_versions_created_by_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "safety_policy_versions" ADD CONSTRAINT "safety_policy_versions_published_by_fk" FOREIGN KEY ("published_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_runs" ADD CONSTRAINT "task_runs_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_runs" ADD CONSTRAINT "task_runs_task_id_tasks_id_fk" FOREIGN KEY ("task_id") REFERENCES "public"."tasks"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_runs" ADD CONSTRAINT "task_runs_created_by_user_id_users_id_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_runs" ADD CONSTRAINT "task_runs_version_project_fk" FOREIGN KEY ("task_version_id","project_id") REFERENCES "public"."task_versions"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_runs" ADD CONSTRAINT "task_runs_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_runs" ADD CONSTRAINT "task_runs_device_project_fk" FOREIGN KEY ("selected_device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE SET NULL ("selected_device_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_runs" ADD CONSTRAINT "task_runs_policy_project_fk" FOREIGN KEY ("safety_policy_version_id","project_id") REFERENCES "public"."safety_policy_versions"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_runs" ADD CONSTRAINT "task_runs_approval_project_fk" FOREIGN KEY ("approval_request_id","project_id") REFERENCES "public"."approval_requests"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_run_steps" ADD CONSTRAINT "task_run_steps_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_run_steps" ADD CONSTRAINT "task_run_steps_run_project_fk" FOREIGN KEY ("task_run_id","project_id") REFERENCES "public"."task_runs"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_run_steps" ADD CONSTRAINT "task_run_steps_step_project_fk" FOREIGN KEY ("task_step_id","project_id") REFERENCES "public"."task_steps"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "tasks" ADD CONSTRAINT "tasks_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "tasks" ADD CONSTRAINT "tasks_created_by_user_id_users_id_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "tasks" ADD CONSTRAINT "tasks_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "tasks" ADD CONSTRAINT "tasks_current_version_project_fk" FOREIGN KEY ("current_published_version_id","project_id") REFERENCES "public"."task_versions"("id","project_id") ON DELETE SET NULL ("current_published_version_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_versions" ADD CONSTRAINT "task_versions_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_versions" ADD CONSTRAINT "task_versions_task_project_fk" FOREIGN KEY ("task_id","project_id") REFERENCES "public"."tasks"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_versions" ADD CONSTRAINT "task_versions_created_by_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_versions" ADD CONSTRAINT "task_versions_published_by_fk" FOREIGN KEY ("published_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_steps" ADD CONSTRAINT "task_steps_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_steps" ADD CONSTRAINT "task_steps_version_project_fk" FOREIGN KEY ("task_version_id","project_id") REFERENCES "public"."task_versions"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "team_members" ADD CONSTRAINT "team_members_team_id_teams_id_fk" FOREIGN KEY ("team_id") REFERENCES "public"."teams"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "team_members" ADD CONSTRAINT "team_members_user_id_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "agent_messages_session_created_idx" ON "agent_messages" USING btree ("session_id","created_at");--> statement-breakpoint
CREATE INDEX "agent_sessions_project_created_idx" ON "agent_sessions" USING btree ("project_id","created_at");--> statement-breakpoint
CREATE INDEX "agent_sessions_issue_idx" ON "agent_sessions" USING btree ("issue_id");--> statement-breakpoint
CREATE INDEX "agent_sessions_task_run_idx" ON "agent_sessions" USING btree ("task_run_id");--> statement-breakpoint
CREATE UNIQUE INDEX "agents_project_name_unique" ON "agents" USING btree ("project_id","name");--> statement-breakpoint
CREATE INDEX "agents_project_status_idx" ON "agents" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "algorithm_providers_project_status_idx" ON "algorithm_providers" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "algorithm_definitions_project_provider_idx" ON "algorithm_definitions" USING btree ("project_id","provider_id");--> statement-breakpoint
CREATE UNIQUE INDEX "algorithm_definition_versions_one_draft_idx" ON "algorithm_definition_versions" USING btree ("algorithm_definition_id") WHERE "algorithm_definition_versions"."status" = 'draft';--> statement-breakpoint
CREATE INDEX "algorithm_definition_versions_project_status_idx" ON "algorithm_definition_versions" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "algorithm_runs_claim_idx" ON "algorithm_runs" USING btree ("status","created_at");--> statement-breakpoint
CREATE INDEX "algorithm_runs_project_created_idx" ON "algorithm_runs" USING btree ("project_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "algorithm_run_attempts_run_idx" ON "algorithm_run_attempts" USING btree ("algorithm_run_id","attempt");--> statement-breakpoint
CREATE INDEX "approval_requests_project_status_idx" ON "approval_requests" USING btree ("project_id","status","expires_at");--> statement-breakpoint
CREATE INDEX "approvals_request_decided_idx" ON "approvals" USING btree ("approval_request_id","decided_at");--> statement-breakpoint
CREATE INDEX "assets_project_created_idx" ON "assets" USING btree ("project_id","created_at");--> statement-breakpoint
CREATE INDEX "assets_task_run_idx" ON "assets" USING btree ("task_run_id");--> statement-breakpoint
CREATE INDEX "assets_issue_idx" ON "assets" USING btree ("issue_id");--> statement-breakpoint
CREATE INDEX "assets_device_idx" ON "assets" USING btree ("device_id");--> statement-breakpoint
CREATE INDEX "assets_project_status_created_idx" ON "assets" USING btree ("project_id","status","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "assets_retention_idx" ON "assets" USING btree ("project_id","retention_hold_until") WHERE "assets"."status" = 'available';--> statement-breakpoint
CREATE INDEX "asset_upload_intents_expiry_idx" ON "asset_upload_intents" USING btree ("status","expires_at");--> statement-breakpoint
CREATE INDEX "asset_derivatives_source_idx" ON "asset_derivatives" USING btree ("project_id","source_asset_id");--> statement-breakpoint
CREATE INDEX "evidence_links_target_idx" ON "evidence_links" USING btree ("project_id","target_type","target_id");--> statement-breakpoint
CREATE UNIQUE INDEX "live_streams_one_active_device_key_idx" ON "live_streams" USING btree ("project_id","device_id","stream_key") WHERE "live_streams"."status" in ('starting', 'live', 'degraded', 'stopping');--> statement-breakpoint
CREATE INDEX "live_streams_project_status_idx" ON "live_streams" USING btree ("project_id","status","started_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "live_streams_device_started_idx" ON "live_streams" USING btree ("device_id","started_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "audit_events_project_created_idx" ON "audit_events" USING btree ("project_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "audit_events_request_idx" ON "audit_events" USING btree ("request_id");--> statement-breakpoint
CREATE INDEX "audit_events_resource_idx" ON "audit_events" USING btree ("project_id","resource_type","resource_id");--> statement-breakpoint
CREATE INDEX "device_adapters_project_status_idx" ON "device_adapters" USING btree ("project_id","status");--> statement-breakpoint
CREATE UNIQUE INDEX "device_capabilities_device_code_unique" ON "device_capabilities" USING btree ("device_id","capability_code");--> statement-breakpoint
CREATE INDEX "device_capabilities_code_idx" ON "device_capabilities" USING btree ("capability_code");--> statement-breakpoint
CREATE UNIQUE INDEX "devices_project_name_unique" ON "devices" USING btree ("project_id","name");--> statement-breakpoint
CREATE INDEX "devices_project_status_idx" ON "devices" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "devices_last_seen_idx" ON "devices" USING btree ("last_seen_at");--> statement-breakpoint
CREATE INDEX "device_external_identities_project_idx" ON "device_external_identities" USING btree ("project_id","last_seen_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "device_connections_project_status_idx" ON "device_connections" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "device_commands_dispatch_idx" ON "device_commands" USING btree ("status","priority" DESC NULLS LAST,"deadline_at");--> statement-breakpoint
CREATE INDEX "device_commands_run_created_idx" ON "device_commands" USING btree ("task_run_id","created_at");--> statement-breakpoint
CREATE INDEX "command_attempts_command_idx" ON "command_attempts" USING btree ("command_id","attempt");--> statement-breakpoint
CREATE INDEX "device_connections_device_opened_idx" ON "device_connections" USING btree ("device_id","opened_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "device_connections_open_heartbeat_idx" ON "device_connections" USING btree ("last_heartbeat_at") WHERE "device_connections"."closed_at" is null;--> statement-breakpoint
CREATE UNIQUE INDEX "device_telemetry_source_event_unique" ON "device_telemetry" USING btree ("adapter_id","event_id","captured_at");--> statement-breakpoint
CREATE INDEX "device_telemetry_project_time_idx" ON "device_telemetry" USING btree ("project_id","captured_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "device_telemetry_device_time_idx" ON "device_telemetry" USING btree ("device_id","captured_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "telemetry_event_dedup_received_idx" ON "telemetry_event_dedup" USING btree ("received_at");--> statement-breakpoint
CREATE INDEX "device_latest_telemetry_project_time_idx" ON "device_latest_telemetry" USING btree ("project_id","captured_at" DESC NULLS LAST);--> statement-breakpoint
CREATE UNIQUE INDEX "coordinate_references_one_standard_idx" ON "coordinate_references" USING btree ("project_id") WHERE "coordinate_references"."is_project_standard";--> statement-breakpoint
CREATE INDEX "sensor_calibrations_device_valid_idx" ON "sensor_calibrations" USING btree ("device_id","sensor_key","valid_from" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "observations_project_time_idx" ON "observations" USING btree ("project_id","captured_at" DESC NULLS LAST,"id");--> statement-breakpoint
CREATE INDEX "observations_device_type_time_idx" ON "observations" USING btree ("device_id","observation_type","captured_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "observations_standard_geometry_gist" ON "observations" USING gist ("standard_geometry");--> statement-breakpoint
CREATE INDEX "observations_original_geometry_gist" ON "observations" USING gist ("original_geometry");--> statement-breakpoint
CREATE INDEX "poses_project_time_idx" ON "poses" USING btree ("project_id","captured_at" DESC NULLS LAST,"observation_id");--> statement-breakpoint
CREATE INDEX "poses_device_time_idx" ON "poses" USING btree ("device_id","captured_at" DESC NULLS LAST,"observation_id");--> statement-breakpoint
CREATE INDEX "poses_standard_position_gist" ON "poses" USING gist ("standard_position");--> statement-breakpoint
CREATE INDEX "issue_events_issue_created_idx" ON "issue_events" USING btree ("issue_id","created_at");--> statement-breakpoint
CREATE INDEX "issue_events_project_created_idx" ON "issue_events" USING btree ("project_id","created_at");--> statement-breakpoint
CREATE INDEX "idempotency_records_expiry_idx" ON "idempotency_records" USING btree ("expires_at");--> statement-breakpoint
CREATE INDEX "idempotency_records_project_created_idx" ON "idempotency_records" USING btree ("project_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE UNIQUE INDEX "issue_links_issue_target_unique" ON "issue_links" USING btree ("issue_id","link_type","target_id");--> statement-breakpoint
CREATE INDEX "issue_links_target_idx" ON "issue_links" USING btree ("link_type","target_id");--> statement-breakpoint
CREATE INDEX "issue_links_project_issue_idx" ON "issue_links" USING btree ("project_id","issue_id");--> statement-breakpoint
CREATE UNIQUE INDEX "issues_project_number_unique" ON "issues" USING btree ("project_id","number");--> statement-breakpoint
CREATE INDEX "issues_project_status_idx" ON "issues" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "issues_project_priority_idx" ON "issues" USING btree ("project_id","priority");--> statement-breakpoint
CREATE INDEX "issues_task_run_idx" ON "issues" USING btree ("task_run_id");--> statement-breakpoint
CREATE INDEX "outbox_events_claim_idx" ON "outbox_events" USING btree ("status","available_at","locked_until","id");--> statement-breakpoint
CREATE INDEX "outbox_events_project_created_idx" ON "outbox_events" USING btree ("project_id","created_at","id");--> statement-breakpoint
CREATE UNIQUE INDEX "projects_team_name_unique" ON "projects" USING btree ("team_id","name");--> statement-breakpoint
CREATE INDEX "projects_team_idx" ON "projects" USING btree ("team_id");--> statement-breakpoint
CREATE INDEX "project_feature_flags_updated_idx" ON "project_feature_flags" USING btree ("updated_at");--> statement-breakpoint
CREATE INDEX "project_permissions_user_project_idx" ON "project_permissions" USING btree ("user_id","project_id");--> statement-breakpoint
CREATE INDEX "project_permissions_project_permission_idx" ON "project_permissions" USING btree ("project_id","permission");--> statement-breakpoint
CREATE INDEX "project_events_project_cursor_idx" ON "project_events" USING btree ("project_id","cursor");--> statement-breakpoint
CREATE INDEX "project_events_project_occurred_idx" ON "project_events" USING btree ("project_id","occurred_at","cursor");--> statement-breakpoint
CREATE UNIQUE INDEX "safety_policy_versions_one_draft_idx" ON "safety_policy_versions" USING btree ("project_id") WHERE "safety_policy_versions"."status" = 'draft';--> statement-breakpoint
CREATE INDEX "safety_policy_versions_boundary_gist" ON "safety_policy_versions" USING gist ("project_boundary");--> statement-breakpoint
CREATE INDEX "safety_policy_versions_restricted_gist" ON "safety_policy_versions" USING gist ("restricted_areas");--> statement-breakpoint
CREATE INDEX "task_runs_project_created_idx" ON "task_runs" USING btree ("project_id","created_at");--> statement-breakpoint
CREATE INDEX "task_runs_task_created_idx" ON "task_runs" USING btree ("task_id","created_at");--> statement-breakpoint
CREATE INDEX "task_runs_status_idx" ON "task_runs" USING btree ("status");--> statement-breakpoint
CREATE INDEX "task_run_steps_run_status_idx" ON "task_run_steps" USING btree ("task_run_id","status","position");--> statement-breakpoint
CREATE UNIQUE INDEX "tasks_project_name_unique" ON "tasks" USING btree ("project_id","name");--> statement-breakpoint
CREATE INDEX "tasks_project_status_idx" ON "tasks" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "tasks_trigger_type_idx" ON "tasks" USING btree ("trigger_type");--> statement-breakpoint
CREATE UNIQUE INDEX "task_versions_one_draft_idx" ON "task_versions" USING btree ("task_id") WHERE "task_versions"."status" = 'draft';--> statement-breakpoint
CREATE INDEX "task_versions_project_status_idx" ON "task_versions" USING btree ("project_id","status","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "task_steps_version_position_idx" ON "task_steps" USING btree ("task_version_id","position");--> statement-breakpoint
CREATE UNIQUE INDEX "team_members_single_owner_unique" ON "team_members" USING btree ("team_id") WHERE "team_members"."role" = 'owner';--> statement-breakpoint
CREATE INDEX "team_members_user_idx" ON "team_members" USING btree ("user_id");
--> statement-breakpoint
CREATE OR REPLACE FUNCTION notify_aerosight_outbox() RETURNS trigger AS $$
BEGIN
	PERFORM pg_notify('aerosight_outbox', json_build_object('projectId', NEW.project_id, 'eventId', NEW.event_id)::text);
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
--> statement-breakpoint
CREATE TRIGGER outbox_events_notify AFTER INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION notify_aerosight_outbox();
--> statement-breakpoint
CREATE OR REPLACE FUNCTION notify_aerosight_project_event() RETURNS trigger AS $$
BEGIN
	PERFORM pg_notify('aerosight_project_events', json_build_object('projectId', NEW.project_id, 'cursor', NEW.cursor)::text);
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
--> statement-breakpoint
CREATE TRIGGER project_events_notify AFTER INSERT ON project_events FOR EACH ROW EXECUTE FUNCTION notify_aerosight_project_event();
