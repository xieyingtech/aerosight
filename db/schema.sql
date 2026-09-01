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
	"created_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "agent_sessions_id_project_unique" UNIQUE("id","project_id")
);
--> statement-breakpoint
CREATE TABLE "agent_drafts" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"session_id" integer NOT NULL,
	"created_by_user_id" integer NOT NULL,
	"draft_type" text NOT NULL,
	"status" text DEFAULT 'draft' NOT NULL,
	"title" text NOT NULL,
	"payload_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"model_id" text,
	"prompt_template_version" text,
	"generation_tool_calls_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"evidence_version_hash" text,
	"generated_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "agent_drafts_type_valid" CHECK (draft_type in('inspection_task','report','issue')),
	CONSTRAINT "agent_drafts_status_valid" CHECK (status in('draft','discarded','published')),
	CONSTRAINT "agent_drafts_generation_metadata_complete" CHECK ((model_id is null and prompt_template_version is null and evidence_version_hash is null and generated_at is null) or (model_id is not null and prompt_template_version is not null and evidence_version_hash is not null and generated_at is not null)),
	CONSTRAINT "agent_drafts_id_project_unique" UNIQUE("id","project_id")
);
--> statement-breakpoint
CREATE TABLE "agent_draft_evidence" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"agent_draft_id" uuid NOT NULL,
	"reference_type" text NOT NULL,
	"reference_id" text NOT NULL,
	"reference_version" text NOT NULL,
	"observed_at" timestamp with time zone NOT NULL,
	"quality" text NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "agent_draft_evidence_type_valid" CHECK (reference_type in('asset','event','detection','track','task_run')),
	CONSTRAINT "agent_draft_evidence_unique" UNIQUE("agent_draft_id","reference_type","reference_id","reference_version")
);
--> statement-breakpoint
CREATE TABLE "agent_tool_jobs" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"session_id" integer NOT NULL,
	"requested_by_user_id" integer NOT NULL,
	"issue_id" integer,
	"trigger_issue_event_id" integer,
	"trigger_type" text,
	"idempotency_key" text,
	"tool_name" text NOT NULL,
	"required_permission" text NOT NULL,
	"args_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"status" text DEFAULT 'queued' NOT NULL,
	"context_expires_at" timestamp with time zone NOT NULL,
	"authorization_checked_at" timestamp with time zone,
	"started_at" timestamp with time zone,
	"finished_at" timestamp with time zone,
	"failure_code" text,
	"result_json" jsonb,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "agent_tool_jobs_status_valid" CHECK (status in('queued','running','succeeded','failed')),
	CONSTRAINT "agent_tool_jobs_expiry_valid" CHECK (context_expires_at > created_at),
	CONSTRAINT "agent_tool_jobs_trigger_type_valid" CHECK (trigger_type is null or trigger_type in ('issue_mention','issue_assignment','task_step','chat'))
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
	"updated_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "agents_id_project_unique" UNIQUE("id", "project_id")
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
	"task_run_step_id" bigint,
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
CREATE TABLE "algorithm_callback_receipts" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"algorithm_run_id" uuid NOT NULL,
	"provider_id" bigint NOT NULL,
	"callback_id" text NOT NULL,
	"external_job_id" text NOT NULL,
	"payload_hash" text NOT NULL,
	"disposition" text DEFAULT 'verified' NOT NULL,
	"received_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "algorithm_callback_receipts_hash_valid" CHECK (payload_hash ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "algorithm_callback_receipts_disposition_valid" CHECK (disposition in ('verified','applied')),
	CONSTRAINT "algorithm_callback_receipts_provider_callback_unique" UNIQUE("provider_id", "callback_id")
);
--> statement-breakpoint
CREATE TABLE "detections" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"algorithm_run_id" uuid NOT NULL,
	"input_asset_id" integer NOT NULL,
	"task_run_id" integer,
	"detection_key" text NOT NULL,
	"label" text NOT NULL,
	"confidence" double precision NOT NULL,
	"pixel_geometry_json" jsonb NOT NULL,
	"geographic_geometry" geometry(Polygon,4326),
	"location_quality" text DEFAULT 'unavailable' NOT NULL,
	"projection_method" text DEFAULT 'image-only' NOT NULL,
	"horizontal_error_meters" double precision,
	"transform_version" text NOT NULL,
	"attributes_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"captured_at" timestamp with time zone NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "detections_confidence_valid" CHECK (confidence between 0 and 1),
	CONSTRAINT "detections_location_quality_valid" CHECK (location_quality in ('surveyed','estimated','low','unavailable')),
	CONSTRAINT "detections_error_valid" CHECK (horizontal_error_meters is null or horizontal_error_meters >= 0),
	CONSTRAINT "detections_run_key_unique" UNIQUE("algorithm_run_id", "detection_key"),
	CONSTRAINT "detections_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "detection_groups" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"label" text NOT NULL,
	"status" text DEFAULT 'active' NOT NULL,
	"geographic_geometry" geometry(Polygon,4326),
	"location_quality" text NOT NULL,
	"first_detected_at" timestamp with time zone NOT NULL,
	"last_detected_at" timestamp with time zone NOT NULL,
	"member_count" integer DEFAULT 1 NOT NULL,
	"aggregation_version" text DEFAULT 'aerosight-detection-aggregation/v1' NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "detection_groups_status_valid" CHECK (status in ('active','superseded')),
	CONSTRAINT "detection_groups_location_quality_valid" CHECK (location_quality in ('surveyed','estimated','low','unavailable')),
	CONSTRAINT "detection_groups_time_valid" CHECK (last_detected_at >= first_detected_at and member_count > 0),
	CONSTRAINT "detection_groups_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "detection_group_members" (
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"detection_group_id" bigint NOT NULL,
	"detection_id" bigint NOT NULL,
	"added_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "detection_group_members_detection_unique" UNIQUE("detection_id"),
	CONSTRAINT "detection_group_members_pk" PRIMARY KEY("detection_group_id", "detection_id")
);
--> statement-breakpoint
CREATE TABLE "event_rules" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"name" text NOT NULL,
	"status" text DEFAULT 'disabled' NOT NULL,
	"current_published_version_id" bigint,
	"created_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "event_rules_status_valid" CHECK (status in ('disabled','active','retired')),
	CONSTRAINT "event_rules_project_name_unique" UNIQUE("project_id","name"),
	CONSTRAINT "event_rules_id_project_unique" UNIQUE("id","project_id")
);
--> statement-breakpoint
CREATE TABLE "event_rule_versions" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"event_rule_id" bigint NOT NULL,
	"version" integer NOT NULL,
	"status" text DEFAULT 'draft' NOT NULL,
	"label" text NOT NULL,
	"minimum_confidence" double precision NOT NULL,
	"severity" text NOT NULL,
	"deduplication_window_seconds" integer DEFAULT 3600 NOT NULL,
	"conditions_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_by_user_id" integer,
	"published_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"published_at" timestamp with time zone,
	CONSTRAINT "event_rule_versions_status_valid" CHECK (status in ('draft','published','retired')),
	CONSTRAINT "event_rule_versions_confidence_valid" CHECK (minimum_confidence between 0 and 1),
	CONSTRAINT "event_rule_versions_severity_valid" CHECK (severity in ('low','medium','high','critical')),
	CONSTRAINT "event_rule_versions_window_valid" CHECK (deduplication_window_seconds > 0),
	CONSTRAINT "event_rule_versions_rule_version_unique" UNIQUE("event_rule_id","version"),
	CONSTRAINT "event_rule_versions_id_project_unique" UNIQUE("id","project_id")
);
--> statement-breakpoint
CREATE TABLE "perception_events" (
	"id" uuid PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"event_rule_version_id" bigint NOT NULL,
	"detection_group_id" bigint NOT NULL,
	"deduplication_key" text NOT NULL,
	"title" text DEFAULT '疑似违建' NOT NULL,
	"severity" text NOT NULL,
	"status" text DEFAULT 'open' NOT NULL,
	"occurrence_count" integer DEFAULT 1 NOT NULL,
	"state_version" integer DEFAULT 0 NOT NULL,
	"assigned_user_id" integer,
	"first_detected_at" timestamp with time zone NOT NULL,
	"last_detected_at" timestamp with time zone NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"resolved_at" timestamp with time zone,
	CONSTRAINT "perception_events_severity_valid" CHECK (severity in ('low','medium','high','critical')),
	CONSTRAINT "perception_events_status_valid" CHECK (status in ('open','acknowledged','investigating','resolved','dismissed')),
	CONSTRAINT "perception_events_counts_valid" CHECK (occurrence_count > 0 and state_version >= 0 and last_detected_at >= first_detected_at),
	CONSTRAINT "perception_events_id_project_unique" UNIQUE("id","project_id")
);
--> statement-breakpoint
CREATE TABLE "event_feedback" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"perception_event_id" uuid NOT NULL,
	"action" text NOT NULL,
	"value_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"reason" text NOT NULL,
	"actor_user_id" integer NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "event_feedback_action_valid" CHECK (action in ('confirm','false_positive','category_correction','assign','acknowledge','investigate','dismiss','resolve'))
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
	"session_token" uuid DEFAULT gen_random_uuid() NOT NULL,
	"playback_ref" text,
	"playback_locator_expires_at" timestamp with time zone,
	"status_reason" text,
	"vendor_stream_ref" text,
	"started_by_user_id" integer,
	"started_at" timestamp with time zone DEFAULT now() NOT NULL,
	"last_active_at" timestamp with time zone,
	"ended_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "live_streams_status_valid" CHECK (status in ('requested', 'starting', 'live', 'degraded', 'failed', 'stopping', 'stopped')),
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
	"uav_registration_number" text,
	"registration_valid_until" timestamp with time zone,
	"remote_identification_code" text,
	"responsible_user_id" integer,
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
	"client_key" text,
	"created_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "issue_events_id_project_unique" UNIQUE("id", "project_id")
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
	"target_id" text NOT NULL,
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
	"task_version_id" bigint,
	"condition_scope_key" text,
	"business_object_key" text,
	"occurrence_count" integer DEFAULT 1 NOT NULL,
	"first_seen_at" timestamp with time zone DEFAULT now() NOT NULL,
	"last_seen_at" timestamp with time zone DEFAULT now() NOT NULL,
	"labels_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"state_version" integer DEFAULT 0 NOT NULL,
	"opened_by_user_id" integer,
	"assignee_user_id" integer,
	"closed_at" timestamp,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "issues_id_project_unique" UNIQUE("id", "project_id"),
	CONSTRAINT "issues_occurrence_positive" CHECK (occurrence_count > 0),
	CONSTRAINT "issues_labels_array" CHECK (jsonb_typeof(labels_json) = 'array'),
	CONSTRAINT "issues_state_version_nonnegative" CHECK (state_version >= 0)
);
--> statement-breakpoint
CREATE TABLE "issue_assignees" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"issue_id" integer NOT NULL,
	"assignee_type" text NOT NULL,
	"user_id" integer,
	"agent_id" integer,
	"assigned_by_user_id" integer NOT NULL,
	"active" boolean DEFAULT true NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"removed_at" timestamp with time zone,
	CONSTRAINT "issue_assignees_type_valid" CHECK (assignee_type in ('user','agent')),
	CONSTRAINT "issue_assignees_subject_valid" CHECK ((assignee_type='user' and user_id is not null and agent_id is null) or (assignee_type='agent' and agent_id is not null and user_id is null)),
	CONSTRAINT "issue_assignees_active_time_valid" CHECK (active=(removed_at is null))
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
	"operation_approval_reference" text,
	"operation_approval_valid_until" timestamp with time zone,
	"takeoff_confirmed_at" timestamp with time zone,
	"takeoff_confirmed_by_user_id" integer,
	"responsible_user_id" integer,
	"incident_report_reference" text,
	"incident_reported_at" timestamp with time zone,
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
	CONSTRAINT "task_runs_current_step_valid" CHECK (current_step_position is null or current_step_position > 0),
	CONSTRAINT "task_runs_takeoff_confirmation_complete" CHECK ((takeoff_confirmed_at is null) = (takeoff_confirmed_by_user_id is null)),
	CONSTRAINT "task_runs_incident_report_complete" CHECK ((incident_report_reference is null) = (incident_reported_at is null))
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
	"input_snapshot_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"output_snapshot_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"condition_result_json" jsonb,
	"execution_key" text,
	"started_at" timestamp with time zone,
	"finished_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "task_run_steps_status_valid" CHECK (status in ('pending','dispatching','running','succeeded','failed','skipped','paused')),
	CONSTRAINT "task_run_steps_position_valid" CHECK (position > 0 and attempt_count >= 0),
	CONSTRAINT "task_run_steps_input_object" CHECK (jsonb_typeof(input_snapshot_json) = 'object'),
	CONSTRAINT "task_run_steps_output_object" CHECK (jsonb_typeof(output_snapshot_json) = 'object'),
	CONSTRAINT "task_run_steps_condition_object" CHECK (condition_result_json is null or jsonb_typeof(condition_result_json) = 'object'),
	CONSTRAINT "task_run_steps_project_execution_unique" UNIQUE("project_id", "execution_key"),
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
	"input_schema_json" jsonb DEFAULT '{"type":"object","properties":{},"additionalProperties":false}'::jsonb NOT NULL,
	"trigger_json" jsonb DEFAULT '{"type":"manual"}'::jsonb NOT NULL,
	"concurrency_limit" integer DEFAULT 1 NOT NULL,
	"created_by_user_id" integer,
	"published_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"published_at" timestamp with time zone,
	CONSTRAINT "task_versions_status_valid" CHECK (status in ('draft', 'published', 'retired')),
	CONSTRAINT "task_versions_version_positive" CHECK (version > 0),
	CONSTRAINT "task_versions_input_schema_object" CHECK (jsonb_typeof(input_schema_json) = 'object'),
	CONSTRAINT "task_versions_trigger_object" CHECK (jsonb_typeof(trigger_json) = 'object'),
	CONSTRAINT "task_versions_concurrency_positive" CHECK (concurrency_limit > 0 and concurrency_limit <= 100),
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
	"uses" text DEFAULT 'device.command' NOT NULL,
	"input_schema_json" jsonb DEFAULT '{"type":"object","properties":{}}'::jsonb NOT NULL,
	"output_schema_json" jsonb DEFAULT '{"type":"object","properties":{}}'::jsonb NOT NULL,
	"condition_json" jsonb,
	"depends_on_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"timeout_seconds" integer DEFAULT 300 NOT NULL,
	"retry_policy_json" jsonb DEFAULT '{"maxAttempts":1,"backoffSeconds":0}'::jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "task_steps_position_positive" CHECK (position > 0),
	CONSTRAINT "task_steps_uses_valid" CHECK (uses in ('device.command','device.collect','algorithm.run','issue.create-or-update','copilot.run','report.generate')),
	CONSTRAINT "task_steps_input_schema_object" CHECK (jsonb_typeof(input_schema_json) = 'object'),
	CONSTRAINT "task_steps_output_schema_object" CHECK (jsonb_typeof(output_schema_json) = 'object'),
	CONSTRAINT "task_steps_condition_object" CHECK (condition_json is null or jsonb_typeof(condition_json) = 'object'),
	CONSTRAINT "task_steps_depends_array" CHECK (jsonb_typeof(depends_on_json) = 'array'),
	CONSTRAINT "task_steps_timeout_positive" CHECK (timeout_seconds > 0 and timeout_seconds <= 86400),
	CONSTRAINT "task_steps_retry_policy_object" CHECK (jsonb_typeof(retry_policy_json) = 'object'),
	CONSTRAINT "task_steps_version_position_unique" UNIQUE("task_version_id", "position"),
	CONSTRAINT "task_steps_version_key_unique" UNIQUE("task_version_id", "step_key"),
	CONSTRAINT "task_steps_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE TABLE "device_commands" (
	"id" uuid PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"task_run_id" integer,
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
CREATE TABLE "device_command_protocol_correlations" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"command_id" uuid NOT NULL,
	"adapter_id" bigint NOT NULL,
	"mapping_version" text NOT NULL,
	"transaction_id" text NOT NULL,
	"business_id" text NOT NULL,
	"method" text NOT NULL,
	"request_topic" text NOT NULL,
	"request_payload_json" jsonb NOT NULL,
	"status" text DEFAULT 'prepared' NOT NULL,
	"reply_event_id" text,
	"reply_result" integer,
	"reply_payload_json" jsonb,
	"sent_at" timestamp with time zone,
	"replied_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "device_command_protocol_correlations_project_team_fk" FOREIGN KEY ("project_id", "team_id") REFERENCES "projects"("id", "team_id") ON DELETE cascade,
	CONSTRAINT "device_command_protocol_correlations_command_project_fk" FOREIGN KEY ("command_id", "project_id") REFERENCES "device_commands"("id", "project_id") ON DELETE cascade,
	CONSTRAINT "device_command_protocol_correlations_adapter_project_fk" FOREIGN KEY ("adapter_id", "project_id") REFERENCES "device_adapters"("id", "project_id") ON DELETE cascade,
	CONSTRAINT "device_command_protocol_correlations_status_valid" CHECK (status in ('prepared','sent','acknowledged','nacked','unknown')),
	CONSTRAINT "device_command_protocol_correlations_command_unique" UNIQUE("command_id"),
	CONSTRAINT "device_command_protocol_correlations_transaction_unique" UNIQUE("adapter_id", "transaction_id"),
	CONSTRAINT "device_command_protocol_correlations_business_method_unique" UNIQUE("adapter_id", "business_id", "method")
);
--> statement-breakpoint
CREATE INDEX "device_command_protocol_correlations_reply_idx"
ON "device_command_protocol_correlations" ("adapter_id", "transaction_id", "business_id", "method", "status");
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
ALTER TABLE "device_adapters"
	ADD COLUMN "lease_owner" text,
	ADD COLUMN "lease_expires_at" timestamp with time zone,
	ADD COLUMN "connection_epoch" bigint DEFAULT 0 NOT NULL,
	ADD COLUMN "last_connected_at" timestamp with time zone;
--> statement-breakpoint
ALTER TABLE "device_adapters" ADD CONSTRAINT "device_adapters_lease_complete" CHECK (
	("lease_owner" is null and "lease_expires_at" is null)
	or ("lease_owner" is not null and "lease_expires_at" is not null)
);
--> statement-breakpoint
CREATE INDEX "device_adapters_lease_claim_idx"
ON "device_adapters" ("adapter_type", "status", "lease_expires_at")
WHERE "status" in ('connecting', 'connected', 'degraded');
--> statement-breakpoint
CREATE TABLE "device_protocol_messages" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"adapter_id" bigint NOT NULL,
	"gateway_sn" text NOT NULL,
	"device_sn" text NOT NULL,
	"topic" text NOT NULL,
	"route_kind" text NOT NULL,
	"transaction_id" text NOT NULL,
	"business_id" text,
	"method" text,
	"timestamp_ms" bigint NOT NULL,
	"sequence_number" bigint,
	"qos" smallint NOT NULL,
	"duplicate_flag" boolean DEFAULT false NOT NULL,
	"payload_json" jsonb NOT NULL,
	"disposition" text DEFAULT 'accepted' NOT NULL,
	"disposition_reason" text,
	"received_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "device_protocol_messages_project_team_fk" FOREIGN KEY ("project_id", "team_id") REFERENCES "projects"("id", "team_id") ON DELETE cascade,
	CONSTRAINT "device_protocol_messages_adapter_project_fk" FOREIGN KEY ("adapter_id", "project_id") REFERENCES "device_adapters"("id", "project_id") ON DELETE cascade,
	CONSTRAINT "device_protocol_messages_route_valid" CHECK ("route_kind" in ('topology', 'state', 'telemetry', 'event', 'request', 'service_reply')),
	CONSTRAINT "device_protocol_messages_disposition_valid" CHECK ("disposition" in ('accepted', 'out_of_order')),
	CONSTRAINT "device_protocol_messages_adapter_topic_tid_unique" UNIQUE("adapter_id", "topic", "transaction_id")
);
--> statement-breakpoint
CREATE INDEX "device_protocol_messages_project_time_idx" ON "device_protocol_messages" ("project_id", "received_at" DESC);
--> statement-breakpoint
CREATE TABLE "device_protocol_cursors" (
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"adapter_id" bigint NOT NULL,
	"route_key" text NOT NULL,
	"last_timestamp_ms" bigint NOT NULL,
	"last_transaction_id" text NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "device_protocol_cursors_pk" PRIMARY KEY("adapter_id", "route_key"),
	CONSTRAINT "device_protocol_cursors_project_team_fk" FOREIGN KEY ("project_id", "team_id") REFERENCES "projects"("id", "team_id") ON DELETE cascade,
	CONSTRAINT "device_protocol_cursors_adapter_project_fk" FOREIGN KEY ("adapter_id", "project_id") REFERENCES "device_adapters"("id", "project_id") ON DELETE cascade
);
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
CREATE TABLE "alert_automation_policies" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"name" text NOT NULL,
	"current_published_version_id" bigint,
	"created_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "alert_automation_policies_project_name_unique" UNIQUE("project_id","name"),
	CONSTRAINT "alert_automation_policies_id_project_unique" UNIQUE("id","project_id"),
	CONSTRAINT "alert_automation_policies_project_team_fk" FOREIGN KEY("project_id","team_id") REFERENCES "projects"("id","team_id") ON DELETE cascade
);
--> statement-breakpoint
CREATE TABLE "alert_automation_policy_versions" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"alert_automation_policy_id" bigint NOT NULL,
	"event_rule_version_id" bigint,
	"version" integer NOT NULL,
	"status" text DEFAULT 'draft' NOT NULL,
	"mode" text DEFAULT 'manual' NOT NULL,
	"config_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_by_user_id" integer,
	"published_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"published_at" timestamp with time zone,
	CONSTRAINT "alert_automation_policy_versions_status_valid" CHECK(status in('draft','published','retired')),
	CONSTRAINT "alert_automation_policy_versions_mode_valid" CHECK(mode in('manual','agent-on-demand','agent-auto-draft','follow-up-draft')),
	CONSTRAINT "alert_automation_policy_versions_version_positive" CHECK(version>0),
	CONSTRAINT "alert_automation_policy_versions_policy_version_unique" UNIQUE("alert_automation_policy_id","version"),
	CONSTRAINT "alert_automation_policy_versions_id_project_unique" UNIQUE("id","project_id"),
	CONSTRAINT "alert_automation_policy_versions_policy_project_fk" FOREIGN KEY("alert_automation_policy_id","project_id") REFERENCES "alert_automation_policies"("id","project_id") ON DELETE cascade,
	CONSTRAINT "alert_automation_policy_versions_rule_project_fk" FOREIGN KEY("event_rule_version_id","project_id") REFERENCES "event_rule_versions"("id","project_id") ON DELETE restrict,
	CONSTRAINT "alert_automation_policy_versions_project_team_fk" FOREIGN KEY("project_id","team_id") REFERENCES "projects"("id","team_id") ON DELETE cascade
);
--> statement-breakpoint
ALTER TABLE "alert_automation_policies" ADD CONSTRAINT "alert_automation_policies_current_version_project_fk" FOREIGN KEY("current_published_version_id","project_id") REFERENCES "alert_automation_policy_versions"("id","project_id") ON DELETE set null;
--> statement-breakpoint
CREATE TABLE "alert_automation_runs" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"policy_version_id" bigint NOT NULL,
	"perception_event_id" uuid NOT NULL,
	"trigger_reason" text NOT NULL,
	"status" text DEFAULT 'queued' NOT NULL,
	"input_scope_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"output_refs_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"failure_code" text,
	"failure_message" text,
	"queued_at" timestamp with time zone DEFAULT now() NOT NULL,
	"started_at" timestamp with time zone,
	"finished_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "alert_automation_runs_status_valid" CHECK(status in('queued','running','succeeded','failed','canceled')),
	CONSTRAINT "alert_automation_runs_id_project_unique" UNIQUE("id","project_id"),
	CONSTRAINT "alert_automation_runs_policy_project_fk" FOREIGN KEY("policy_version_id","project_id") REFERENCES "alert_automation_policy_versions"("id","project_id") ON DELETE restrict,
	CONSTRAINT "alert_automation_runs_event_project_fk" FOREIGN KEY("perception_event_id","project_id") REFERENCES "perception_events"("id","project_id") ON DELETE restrict,
	CONSTRAINT "alert_automation_runs_project_team_fk" FOREIGN KEY("project_id","team_id") REFERENCES "projects"("id","team_id") ON DELETE cascade
);
--> statement-breakpoint
CREATE TABLE "alert_automation_drafts" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"automation_run_id" uuid NOT NULL,
	"perception_event_id" uuid NOT NULL,
	"draft_type" text NOT NULL,
	"status" text DEFAULT 'draft' NOT NULL,
	"title" text NOT NULL,
	"payload_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"evidence_refs_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "alert_automation_drafts_type_valid" CHECK(draft_type in('report','issue','follow-up-task')),
	CONSTRAINT "alert_automation_drafts_status_valid" CHECK(status in('draft','discarded','published')),
	CONSTRAINT "alert_automation_drafts_run_type_unique" UNIQUE("automation_run_id","draft_type"),
	CONSTRAINT "alert_automation_drafts_run_project_fk" FOREIGN KEY("automation_run_id","project_id") REFERENCES "alert_automation_runs"("id","project_id") ON DELETE cascade,
	CONSTRAINT "alert_automation_drafts_event_project_fk" FOREIGN KEY("perception_event_id","project_id") REFERENCES "perception_events"("id","project_id") ON DELETE restrict,
	CONSTRAINT "alert_automation_drafts_project_team_fk" FOREIGN KEY("project_id","team_id") REFERENCES "projects"("id","team_id") ON DELETE cascade
);
--> statement-breakpoint
CREATE TABLE "generated_reports" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"source_type" text NOT NULL,
	"source_id" text NOT NULL,
	"title" text NOT NULL,
	"current_published_version_id" uuid,
	"created_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "generated_reports_source_type_valid" CHECK(source_type in('task_run','perception_event')),
	CONSTRAINT "generated_reports_project_source_unique" UNIQUE("project_id","source_type","source_id"),
	CONSTRAINT "generated_reports_id_project_unique" UNIQUE("id","project_id"),
	CONSTRAINT "generated_reports_project_team_fk" FOREIGN KEY("project_id","team_id") REFERENCES "projects"("id","team_id") ON DELETE cascade
);
--> statement-breakpoint
CREATE TABLE "generated_report_versions" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"generated_report_id" uuid NOT NULL,
	"version" integer NOT NULL,
	"status" text DEFAULT 'draft' NOT NULL,
	"completeness" text NOT NULL,
	"content_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"data_gaps_json" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"created_by_user_id" integer,
	"published_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"published_at" timestamp with time zone,
	CONSTRAINT "generated_report_versions_status_valid" CHECK(status in('draft','published','retired')),
	CONSTRAINT "generated_report_versions_completeness_valid" CHECK(completeness in('complete','incomplete','failed')),
	CONSTRAINT "generated_report_versions_version_positive" CHECK(version>0),
	CONSTRAINT "generated_report_versions_report_version_unique" UNIQUE("generated_report_id","version"),
	CONSTRAINT "generated_report_versions_id_project_unique" UNIQUE("id","project_id"),
	CONSTRAINT "generated_report_versions_report_project_fk" FOREIGN KEY("generated_report_id","project_id") REFERENCES "generated_reports"("id","project_id") ON DELETE cascade,
	CONSTRAINT "generated_report_versions_project_team_fk" FOREIGN KEY("project_id","team_id") REFERENCES "projects"("id","team_id") ON DELETE cascade
);
--> statement-breakpoint
ALTER TABLE "generated_reports" ADD CONSTRAINT "generated_reports_current_version_project_fk" FOREIGN KEY("current_published_version_id","project_id") REFERENCES "generated_report_versions"("id","project_id") ON DELETE set null;
--> statement-breakpoint
CREATE TABLE "generated_report_evidence" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"report_version_id" uuid NOT NULL,
	"evidence_type" text NOT NULL,
	"evidence_id" text NOT NULL,
	"evidence_version" text NOT NULL,
	"asset_id" integer,
	"checksum_sha256" text,
	"href" text NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "generated_report_evidence_type_valid" CHECK(evidence_type in('task_run','task_version','device','track','step','event','feedback','asset')),
	CONSTRAINT "generated_report_evidence_version_unique" UNIQUE("report_version_id","evidence_type","evidence_id","evidence_version"),
	CONSTRAINT "generated_report_evidence_report_project_fk" FOREIGN KEY("report_version_id","project_id") REFERENCES "generated_report_versions"("id","project_id") ON DELETE cascade,
	CONSTRAINT "generated_report_evidence_asset_project_fk" FOREIGN KEY("asset_id","project_id") REFERENCES "assets"("id","project_id") ON DELETE restrict
);
--> statement-breakpoint
CREATE TABLE "retention_policies" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"policy_key" text NOT NULL,
	"version" integer NOT NULL,
	"status" text DEFAULT 'draft' NOT NULL,
	"retention_days" integer NOT NULL,
	"derivative_retention_days" integer NOT NULL,
	"is_default" boolean DEFAULT false NOT NULL,
	"created_by_user_id" integer,
	"published_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"published_at" timestamp with time zone,
	CONSTRAINT "retention_policies_status_valid" CHECK(status in('draft','published','retired')),
	CONSTRAINT "retention_policies_duration_valid" CHECK(retention_days>0 and derivative_retention_days>0),
	CONSTRAINT "retention_policies_version_valid" CHECK(version>0),
	CONSTRAINT "retention_policies_key_version_unique" UNIQUE("project_id","policy_key","version"),
	CONSTRAINT "retention_policies_id_project_unique" UNIQUE("id","project_id"),
	CONSTRAINT "retention_policies_project_team_fk" FOREIGN KEY("project_id","team_id") REFERENCES "projects"("id","team_id") ON DELETE cascade,
	CONSTRAINT "retention_policies_created_by_fk" FOREIGN KEY("created_by_user_id") REFERENCES "users"("id") ON DELETE set null,
	CONSTRAINT "retention_policies_published_by_fk" FOREIGN KEY("published_by_user_id") REFERENCES "users"("id") ON DELETE set null
);
--> statement-breakpoint
CREATE TABLE "retention_holds" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"asset_id" integer NOT NULL,
	"reason" text NOT NULL,
	"status" text DEFAULT 'active' NOT NULL,
	"hold_until" timestamp with time zone,
	"created_by_user_id" integer,
	"released_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"released_at" timestamp with time zone,
	CONSTRAINT "retention_holds_status_valid" CHECK(status in('active','released')),
	CONSTRAINT "retention_holds_release_complete" CHECK((status='active' and released_at is null) or (status='released' and released_at is not null)),
	CONSTRAINT "retention_holds_reason_present" CHECK(length(trim(reason))>0),
	CONSTRAINT "retention_holds_project_team_fk" FOREIGN KEY("project_id","team_id") REFERENCES "projects"("id","team_id") ON DELETE cascade,
	CONSTRAINT "retention_holds_asset_project_fk" FOREIGN KEY("asset_id","project_id") REFERENCES "assets"("id","project_id") ON DELETE cascade,
	CONSTRAINT "retention_holds_created_by_fk" FOREIGN KEY("created_by_user_id") REFERENCES "users"("id") ON DELETE set null,
	CONSTRAINT "retention_holds_released_by_fk" FOREIGN KEY("released_by_user_id") REFERENCES "users"("id") ON DELETE set null
);
--> statement-breakpoint
CREATE TABLE "retention_cleanup_runs" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"retention_policy_id" bigint NOT NULL,
	"mode" text DEFAULT 'dry_run' NOT NULL,
	"status" text DEFAULT 'planned' NOT NULL,
	"plan_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"candidate_count" integer DEFAULT 0 NOT NULL,
	"deleted_count" integer DEFAULT 0 NOT NULL,
	"created_by_user_id" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"completed_at" timestamp with time zone,
	"error_code" text,
	CONSTRAINT "retention_cleanup_runs_mode_valid" CHECK(mode in('dry_run','execute')),
	CONSTRAINT "retention_cleanup_runs_status_valid" CHECK(status in('planned','running','completed','failed')),
	CONSTRAINT "retention_cleanup_runs_counts_valid" CHECK(candidate_count>=0 and deleted_count>=0 and deleted_count<=candidate_count),
	CONSTRAINT "retention_cleanup_runs_id_project_unique" UNIQUE("id","project_id"),
	CONSTRAINT "retention_cleanup_runs_project_team_fk" FOREIGN KEY("project_id","team_id") REFERENCES "projects"("id","team_id") ON DELETE cascade,
	CONSTRAINT "retention_cleanup_runs_policy_project_fk" FOREIGN KEY("retention_policy_id","project_id") REFERENCES "retention_policies"("id","project_id") ON DELETE restrict,
	CONSTRAINT "retention_cleanup_runs_created_by_fk" FOREIGN KEY("created_by_user_id") REFERENCES "users"("id") ON DELETE set null
);
--> statement-breakpoint
CREATE TABLE "retention_deletion_tombstones" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"cleanup_run_id" uuid NOT NULL,
	"retention_policy_id" bigint NOT NULL,
	"asset_id" integer NOT NULL,
	"storage_key_hash" text NOT NULL,
	"checksum_sha256" text,
	"reason_code" text NOT NULL,
	"deleted_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "retention_tombstones_storage_hash_valid" CHECK(storage_key_hash ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "retention_tombstones_checksum_valid" CHECK(checksum_sha256 is null or checksum_sha256 ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "retention_tombstones_asset_unique" UNIQUE("asset_id"),
	CONSTRAINT "retention_tombstones_project_team_fk" FOREIGN KEY("project_id","team_id") REFERENCES "projects"("id","team_id") ON DELETE cascade,
	CONSTRAINT "retention_tombstones_run_project_fk" FOREIGN KEY("cleanup_run_id","project_id") REFERENCES "retention_cleanup_runs"("id","project_id") ON DELETE restrict,
	CONSTRAINT "retention_tombstones_policy_project_fk" FOREIGN KEY("retention_policy_id","project_id") REFERENCES "retention_policies"("id","project_id") ON DELETE restrict,
	CONSTRAINT "retention_tombstones_asset_project_fk" FOREIGN KEY("asset_id","project_id") REFERENCES "assets"("id","project_id") ON DELETE restrict
);
--> statement-breakpoint
CREATE FUNCTION protect_published_retention_policy() RETURNS trigger LANGUAGE plpgsql AS $$
begin
  if old.status='published' then
    raise exception 'published retention policy is immutable' using errcode='55000';
  end if;
  return case when tg_op='DELETE' then old else new end;
end;
$$;
--> statement-breakpoint
CREATE TRIGGER retention_policies_published_immutable BEFORE UPDATE OR DELETE ON retention_policies FOR EACH ROW EXECUTE FUNCTION protect_published_retention_policy();
--> statement-breakpoint
ALTER TABLE "agent_messages" ADD CONSTRAINT "agent_messages_session_id_agent_sessions_id_fk" FOREIGN KEY ("session_id") REFERENCES "public"."agent_sessions"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_agent_id_agents_id_fk" FOREIGN KEY ("agent_id") REFERENCES "public"."agents"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_task_run_id_task_runs_id_fk" FOREIGN KEY ("task_run_id") REFERENCES "public"."task_runs"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_issue_id_issues_id_fk" FOREIGN KEY ("issue_id") REFERENCES "public"."issues"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_issue_project_fk" FOREIGN KEY ("issue_id","project_id") REFERENCES "public"."issues"("id","project_id") ON DELETE SET NULL ("issue_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_agent_project_fk" FOREIGN KEY ("agent_id","project_id") REFERENCES "public"."agents"("id","project_id") ON DELETE SET NULL ("agent_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_sessions" ADD CONSTRAINT "agent_sessions_started_by_user_id_users_id_fk" FOREIGN KEY ("started_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_drafts" ADD CONSTRAINT "agent_drafts_session_project_fk" FOREIGN KEY ("session_id","project_id") REFERENCES "public"."agent_sessions"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_drafts" ADD CONSTRAINT "agent_drafts_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_drafts" ADD CONSTRAINT "agent_drafts_actor_team_fk" FOREIGN KEY ("team_id","created_by_user_id") REFERENCES "public"."team_members"("team_id","user_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_draft_evidence" ADD CONSTRAINT "agent_draft_evidence_draft_fk" FOREIGN KEY ("agent_draft_id","project_id") REFERENCES "public"."agent_drafts"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_tool_jobs" ADD CONSTRAINT "agent_tool_jobs_session_project_fk" FOREIGN KEY ("session_id","project_id") REFERENCES "public"."agent_sessions"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_tool_jobs" ADD CONSTRAINT "agent_tool_jobs_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_tool_jobs" ADD CONSTRAINT "agent_tool_jobs_issue_project_fk" FOREIGN KEY ("issue_id","project_id") REFERENCES "public"."issues"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_tool_jobs" ADD CONSTRAINT "agent_tool_jobs_trigger_event_project_fk" FOREIGN KEY ("trigger_issue_event_id","project_id") REFERENCES "public"."issue_events"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
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
ALTER TABLE "algorithm_runs" ADD CONSTRAINT "algorithm_runs_task_step_project_fk" FOREIGN KEY ("task_run_step_id","project_id") REFERENCES "public"."task_run_steps"("id","project_id") ON DELETE SET NULL ("task_run_step_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_runs" ADD CONSTRAINT "algorithm_runs_device_project_fk" FOREIGN KEY ("device_id","project_id") REFERENCES "public"."devices"("id","project_id") ON DELETE SET NULL ("device_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_run_attempts" ADD CONSTRAINT "algorithm_run_attempts_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_run_attempts" ADD CONSTRAINT "algorithm_run_attempts_run_project_fk" FOREIGN KEY ("algorithm_run_id","project_id") REFERENCES "public"."algorithm_runs"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_callback_receipts" ADD CONSTRAINT "algorithm_callback_receipts_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_callback_receipts" ADD CONSTRAINT "algorithm_callback_receipts_run_project_fk" FOREIGN KEY ("algorithm_run_id","project_id") REFERENCES "public"."algorithm_runs"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "algorithm_callback_receipts" ADD CONSTRAINT "algorithm_callback_receipts_provider_project_fk" FOREIGN KEY ("provider_id","project_id") REFERENCES "public"."algorithm_providers"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "detections" ADD CONSTRAINT "detections_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "detections" ADD CONSTRAINT "detections_run_project_fk" FOREIGN KEY ("algorithm_run_id","project_id") REFERENCES "public"."algorithm_runs"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "detections" ADD CONSTRAINT "detections_asset_project_fk" FOREIGN KEY ("input_asset_id","project_id") REFERENCES "public"."assets"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "detections" ADD CONSTRAINT "detections_task_run_project_fk" FOREIGN KEY ("task_run_id","project_id") REFERENCES "public"."task_runs"("id","project_id") ON DELETE SET NULL ("task_run_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "detection_groups" ADD CONSTRAINT "detection_groups_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "detection_group_members" ADD CONSTRAINT "detection_group_members_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "detection_group_members" ADD CONSTRAINT "detection_group_members_group_project_fk" FOREIGN KEY ("detection_group_id","project_id") REFERENCES "public"."detection_groups"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "detection_group_members" ADD CONSTRAINT "detection_group_members_detection_project_fk" FOREIGN KEY ("detection_id","project_id") REFERENCES "public"."detections"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "event_rules" ADD CONSTRAINT "event_rules_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "event_rules" ADD CONSTRAINT "event_rules_creator_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "event_rules" ADD CONSTRAINT "event_rules_current_version_project_fk" FOREIGN KEY ("current_published_version_id","project_id") REFERENCES "public"."event_rule_versions"("id","project_id") ON DELETE SET NULL ("current_published_version_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "event_rule_versions" ADD CONSTRAINT "event_rule_versions_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "event_rule_versions" ADD CONSTRAINT "event_rule_versions_rule_project_fk" FOREIGN KEY ("event_rule_id","project_id") REFERENCES "public"."event_rules"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "event_rule_versions" ADD CONSTRAINT "event_rule_versions_creator_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "event_rule_versions" ADD CONSTRAINT "event_rule_versions_publisher_fk" FOREIGN KEY ("published_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "perception_events" ADD CONSTRAINT "perception_events_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "perception_events" ADD CONSTRAINT "perception_events_rule_version_project_fk" FOREIGN KEY ("event_rule_version_id","project_id") REFERENCES "public"."event_rule_versions"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "perception_events" ADD CONSTRAINT "perception_events_group_project_fk" FOREIGN KEY ("detection_group_id","project_id") REFERENCES "public"."detection_groups"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "perception_events" ADD CONSTRAINT "perception_events_assignee_fk" FOREIGN KEY ("assigned_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "event_feedback" ADD CONSTRAINT "event_feedback_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "event_feedback" ADD CONSTRAINT "event_feedback_event_project_fk" FOREIGN KEY ("perception_event_id","project_id") REFERENCES "public"."perception_events"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "event_feedback" ADD CONSTRAINT "event_feedback_actor_fk" FOREIGN KEY ("actor_user_id") REFERENCES "public"."users"("id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
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
ALTER TABLE "devices" ADD CONSTRAINT "devices_responsible_user_fk" FOREIGN KEY ("responsible_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
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
ALTER TABLE "issue_events" ADD CONSTRAINT "issue_events_issue_project_fk" FOREIGN KEY ("issue_id","project_id") REFERENCES "public"."issues"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_events" ADD CONSTRAINT "issue_events_actor_user_id_users_id_fk" FOREIGN KEY ("actor_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_events" ADD CONSTRAINT "issue_events_actor_agent_id_agents_id_fk" FOREIGN KEY ("actor_agent_id") REFERENCES "public"."agents"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "idempotency_records" ADD CONSTRAINT "idempotency_records_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_links" ADD CONSTRAINT "issue_links_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_links" ADD CONSTRAINT "issue_links_issue_id_issues_id_fk" FOREIGN KEY ("issue_id") REFERENCES "public"."issues"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_links" ADD CONSTRAINT "issue_links_issue_project_fk" FOREIGN KEY ("issue_id","project_id") REFERENCES "public"."issues"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_links" ADD CONSTRAINT "issue_links_created_by_user_id_users_id_fk" FOREIGN KEY ("created_by_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_assignees" ADD CONSTRAINT "issue_assignees_project_team_fk" FOREIGN KEY ("project_id","team_id") REFERENCES "public"."projects"("id","team_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_assignees" ADD CONSTRAINT "issue_assignees_issue_project_fk" FOREIGN KEY ("issue_id","project_id") REFERENCES "public"."issues"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_assignees" ADD CONSTRAINT "issue_assignees_user_team_fk" FOREIGN KEY ("team_id","user_id") REFERENCES "public"."team_members"("team_id","user_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_assignees" ADD CONSTRAINT "issue_assignees_agent_project_fk" FOREIGN KEY ("agent_id","project_id") REFERENCES "public"."agents"("id","project_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issue_assignees" ADD CONSTRAINT "issue_assignees_actor_fk" FOREIGN KEY ("assigned_by_user_id") REFERENCES "public"."users"("id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issues" ADD CONSTRAINT "issues_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issues" ADD CONSTRAINT "issues_task_run_id_task_runs_id_fk" FOREIGN KEY ("task_run_id") REFERENCES "public"."task_runs"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issues" ADD CONSTRAINT "issues_task_run_project_fk" FOREIGN KEY ("task_run_id","project_id") REFERENCES "public"."task_runs"("id","project_id") ON DELETE SET NULL ("task_run_id") ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "issues" ADD CONSTRAINT "issues_task_version_project_fk" FOREIGN KEY ("task_version_id","project_id") REFERENCES "public"."task_versions"("id","project_id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
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
ALTER TABLE "task_runs" ADD CONSTRAINT "task_runs_takeoff_confirmer_fk" FOREIGN KEY ("takeoff_confirmed_by_user_id") REFERENCES "public"."users"("id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "task_runs" ADD CONSTRAINT "task_runs_responsible_user_fk" FOREIGN KEY ("responsible_user_id") REFERENCES "public"."users"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
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
CREATE UNIQUE INDEX "agents_project_copilot_unique" ON "agents" USING btree ("project_id",(config_json->>'kind')) WHERE config_json->>'kind'='copilot';--> statement-breakpoint
CREATE OR REPLACE FUNCTION provision_project_copilot_agent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO agents(project_id,name,description,status,config_json)
  VALUES(NEW.id,'Copilot','项目级 AI 助手，可通过案件评论提及或负责人指派触发。','active',
         '{"kind":"copilot","builtIn":true}'::jsonb);
  RETURN NEW;
END $$;
--> statement-breakpoint
CREATE TRIGGER projects_provision_copilot_agent
  AFTER INSERT ON projects FOR EACH ROW EXECUTE FUNCTION provision_project_copilot_agent();
--> statement-breakpoint
CREATE INDEX "agent_sessions_issue_idx" ON "agent_sessions" USING btree ("issue_id");--> statement-breakpoint
CREATE INDEX "agent_sessions_task_run_idx" ON "agent_sessions" USING btree ("task_run_id");--> statement-breakpoint
CREATE INDEX "agent_drafts_project_created_idx" ON "agent_drafts" USING btree ("project_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "agent_drafts_session_created_idx" ON "agent_drafts" USING btree ("session_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "agent_draft_evidence_project_ref_idx" ON "agent_draft_evidence" USING btree ("project_id","reference_type","reference_id");--> statement-breakpoint
CREATE INDEX "agent_tool_jobs_claim_idx" ON "agent_tool_jobs" USING btree ("status","created_at") WHERE "agent_tool_jobs"."status"='queued';--> statement-breakpoint
CREATE INDEX "agent_tool_jobs_session_idx" ON "agent_tool_jobs" USING btree ("session_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE UNIQUE INDEX "agent_tool_jobs_project_idempotency_unique" ON "agent_tool_jobs" USING btree ("project_id","idempotency_key") WHERE "agent_tool_jobs"."idempotency_key" is not null;--> statement-breakpoint
CREATE INDEX "agent_tool_jobs_issue_created_idx" ON "agent_tool_jobs" USING btree ("issue_id","created_at" DESC NULLS LAST) WHERE "agent_tool_jobs"."issue_id" is not null;--> statement-breakpoint
CREATE UNIQUE INDEX "alert_automation_policy_versions_one_draft_idx" ON "alert_automation_policy_versions" USING btree ("alert_automation_policy_id") WHERE "alert_automation_policy_versions"."status"='draft';--> statement-breakpoint
CREATE INDEX "alert_automation_policy_versions_project_status_idx" ON "alert_automation_policy_versions" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "alert_automation_runs_claim_idx" ON "alert_automation_runs" USING btree ("status","queued_at") WHERE "alert_automation_runs"."status"='queued';--> statement-breakpoint
CREATE INDEX "alert_automation_runs_project_event_idx" ON "alert_automation_runs" USING btree ("project_id","perception_event_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "alert_automation_drafts_project_event_idx" ON "alert_automation_drafts" USING btree ("project_id","perception_event_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE UNIQUE INDEX "generated_report_versions_one_draft_idx" ON "generated_report_versions" USING btree ("generated_report_id") WHERE "generated_report_versions"."status"='draft';--> statement-breakpoint
CREATE INDEX "generated_reports_project_updated_idx" ON "generated_reports" USING btree ("project_id","updated_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "generated_report_evidence_asset_idx" ON "generated_report_evidence" USING btree ("project_id","asset_id") WHERE "generated_report_evidence"."asset_id" is not null;--> statement-breakpoint
CREATE UNIQUE INDEX "retention_policies_one_default_idx" ON "retention_policies" USING btree ("project_id") WHERE "retention_policies"."status" = 'published' and "retention_policies"."is_default";--> statement-breakpoint
CREATE UNIQUE INDEX "retention_holds_one_active_asset_idx" ON "retention_holds" USING btree ("project_id","asset_id") WHERE "retention_holds"."status" = 'active';--> statement-breakpoint
CREATE INDEX "retention_cleanup_runs_project_created_idx" ON "retention_cleanup_runs" USING btree ("project_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "retention_tombstones_project_deleted_idx" ON "retention_deletion_tombstones" USING btree ("project_id","deleted_at" DESC NULLS LAST);--> statement-breakpoint
CREATE UNIQUE INDEX "agents_project_name_unique" ON "agents" USING btree ("project_id","name");--> statement-breakpoint
CREATE INDEX "agents_project_status_idx" ON "agents" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "algorithm_providers_project_status_idx" ON "algorithm_providers" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "algorithm_definitions_project_provider_idx" ON "algorithm_definitions" USING btree ("project_id","provider_id");--> statement-breakpoint
CREATE UNIQUE INDEX "algorithm_definition_versions_one_draft_idx" ON "algorithm_definition_versions" USING btree ("algorithm_definition_id") WHERE "algorithm_definition_versions"."status" = 'draft';--> statement-breakpoint
CREATE INDEX "algorithm_definition_versions_project_status_idx" ON "algorithm_definition_versions" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "algorithm_runs_claim_idx" ON "algorithm_runs" USING btree ("status","created_at");--> statement-breakpoint
CREATE INDEX "algorithm_runs_project_created_idx" ON "algorithm_runs" USING btree ("project_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "algorithm_runs_task_step_idx" ON "algorithm_runs" USING btree ("task_run_step_id") WHERE "algorithm_runs"."task_run_step_id" is not null;--> statement-breakpoint
CREATE INDEX "algorithm_run_attempts_run_idx" ON "algorithm_run_attempts" USING btree ("algorithm_run_id","attempt");--> statement-breakpoint
CREATE INDEX "algorithm_callback_receipts_run_idx" ON "algorithm_callback_receipts" USING btree ("algorithm_run_id","received_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "detections_project_captured_idx" ON "detections" USING btree ("project_id","captured_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "detections_geometry_gist" ON "detections" USING gist ("geographic_geometry");--> statement-breakpoint
CREATE INDEX "detection_groups_project_time_idx" ON "detection_groups" USING btree ("project_id","last_detected_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "detection_groups_geometry_gist" ON "detection_groups" USING gist ("geographic_geometry");--> statement-breakpoint
CREATE UNIQUE INDEX "event_rule_versions_one_draft_idx" ON "event_rule_versions" USING btree ("event_rule_id") WHERE "event_rule_versions"."status" = 'draft';--> statement-breakpoint
CREATE UNIQUE INDEX "perception_events_active_dedup_idx" ON "perception_events" USING btree ("project_id","deduplication_key") WHERE "perception_events"."status" in ('open','acknowledged','investigating');--> statement-breakpoint
CREATE INDEX "perception_events_project_status_idx" ON "perception_events" USING btree ("project_id","status","last_detected_at" DESC NULLS LAST);--> statement-breakpoint
CREATE INDEX "event_feedback_event_created_idx" ON "event_feedback" USING btree ("perception_event_id","created_at");--> statement-breakpoint
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
CREATE UNIQUE INDEX "live_streams_one_active_device_key_idx" ON "live_streams" USING btree ("project_id","device_id","stream_key") WHERE "live_streams"."status" in ('requested', 'starting', 'live', 'degraded', 'stopping');--> statement-breakpoint
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
CREATE INDEX "devices_registration_number_idx" ON "devices" USING btree ("uav_registration_number") WHERE "devices"."uav_registration_number" is not null;--> statement-breakpoint
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
CREATE UNIQUE INDEX "issue_events_client_key_unique" ON "issue_events" USING btree ("project_id","issue_id","client_key") WHERE "issue_events"."client_key" is not null;--> statement-breakpoint
CREATE INDEX "idempotency_records_expiry_idx" ON "idempotency_records" USING btree ("expires_at");--> statement-breakpoint
CREATE INDEX "idempotency_records_project_created_idx" ON "idempotency_records" USING btree ("project_id","created_at" DESC NULLS LAST);--> statement-breakpoint
CREATE UNIQUE INDEX "issue_links_issue_target_unique" ON "issue_links" USING btree ("issue_id","link_type","target_id");--> statement-breakpoint
CREATE INDEX "issue_links_target_idx" ON "issue_links" USING btree ("link_type","target_id");--> statement-breakpoint
CREATE INDEX "issue_links_project_issue_idx" ON "issue_links" USING btree ("project_id","issue_id");--> statement-breakpoint
CREATE UNIQUE INDEX "issue_assignees_active_user_unique" ON "issue_assignees" USING btree ("issue_id","user_id") WHERE "issue_assignees"."active" and "issue_assignees"."user_id" is not null;--> statement-breakpoint
CREATE UNIQUE INDEX "issue_assignees_active_agent_unique" ON "issue_assignees" USING btree ("issue_id","agent_id") WHERE "issue_assignees"."active" and "issue_assignees"."agent_id" is not null;--> statement-breakpoint
CREATE INDEX "issue_assignees_project_issue_idx" ON "issue_assignees" USING btree ("project_id","issue_id") WHERE "issue_assignees"."active";--> statement-breakpoint
CREATE UNIQUE INDEX "issues_project_number_unique" ON "issues" USING btree ("project_id","number");--> statement-breakpoint
CREATE INDEX "issues_project_status_idx" ON "issues" USING btree ("project_id","status");--> statement-breakpoint
CREATE INDEX "issues_project_priority_idx" ON "issues" USING btree ("project_id","priority");--> statement-breakpoint
CREATE INDEX "issues_task_run_idx" ON "issues" USING btree ("task_run_id");--> statement-breakpoint
CREATE UNIQUE INDEX "issues_task_business_unique" ON "issues" USING btree ("project_id","task_version_id","condition_scope_key","business_object_key") WHERE "issues"."task_version_id" is not null and "issues"."condition_scope_key" is not null and "issues"."business_object_key" is not null;--> statement-breakpoint
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
CREATE INDEX "task_runs_responsible_created_idx" ON "task_runs" USING btree ("responsible_user_id","created_at" DESC NULLS LAST) WHERE "task_runs"."responsible_user_id" is not null;--> statement-breakpoint
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
CREATE OR REPLACE FUNCTION protect_published_event_rule_version() RETURNS trigger AS $$
BEGIN
	IF OLD.status IN ('published','retired') THEN
		RAISE EXCEPTION 'published event rule versions are immutable' USING ERRCODE = '55000';
	END IF;
	RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$ LANGUAGE plpgsql;
--> statement-breakpoint
CREATE TRIGGER event_rule_versions_published_immutable BEFORE UPDATE OR DELETE ON event_rule_versions FOR EACH ROW EXECUTE FUNCTION protect_published_event_rule_version();
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
--> statement-breakpoint
CREATE TABLE "driver_definitions" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"driver_key" text NOT NULL,
	"version" text NOT NULL,
	"display_name" text NOT NULL,
	"status" text DEFAULT 'active' NOT NULL,
	"manifest_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "driver_definitions_key_version_unique" UNIQUE("driver_key", "version"),
	CONSTRAINT "driver_definitions_status_valid" CHECK (status in ('active', 'disabled', 'retired')),
	CONSTRAINT "driver_definitions_manifest_object" CHECK (jsonb_typeof(manifest_json) = 'object')
);
--> statement-breakpoint
CREATE TABLE "device_types" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"type_key" text NOT NULL,
	"version" integer NOT NULL,
	"display_name" text NOT NULL,
	"category" text NOT NULL,
	"vendor" text,
	"model" text,
	"driver_definition_id" bigint NOT NULL,
	"driver_version_constraint" text DEFAULT '*' NOT NULL,
	"capability_profile_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"status" text DEFAULT 'active' NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "device_types_key_version_unique" UNIQUE("type_key", "version"),
	CONSTRAINT "device_types_version_positive" CHECK (version > 0),
	CONSTRAINT "device_types_status_valid" CHECK (status in ('active', 'retired')),
	CONSTRAINT "device_types_capability_profile_object" CHECK (jsonb_typeof(capability_profile_json) = 'object')
);
--> statement-breakpoint
INSERT INTO "driver_definitions" ("driver_key", "version", "display_name", "manifest_json")
VALUES ('legacy.static', '1.0.0', 'Legacy static device driver',
        '{"protocols":[],"capabilities":{"state.read":{"risk":"low"}},"streamHandlers":[],"commandHandlers":[]}'::jsonb);
--> statement-breakpoint
INSERT INTO "device_types" (
	"type_key", "version", "display_name", "category", "driver_definition_id",
	"driver_version_constraint", "capability_profile_json"
)
SELECT 'legacy.device', 1, 'Legacy device', 'unknown', "id", '=1.0.0',
       '{"capabilities":{"state.read":{}}}'::jsonb
FROM "driver_definitions" WHERE "driver_key" = 'legacy.static' AND "version" = '1.0.0';
--> statement-breakpoint
INSERT INTO driver_definitions (driver_key, version, display_name, manifest_json)
VALUES (
	'dji.cloud', '1.0.0', 'DJI Cloud API',
	'{"driverKey":"dji.cloud","version":"1.0.0","displayName":"DJI Cloud API","protocols":["mqtt5"],"capabilities":[{"code":"state.read","kind":"read","risk":"low","outputSchema":{"type":"object"}},{"code":"mission.execute","kind":"command","risk":"high","inputSchema":{"type":"object"}},{"code":"mission.cancel","kind":"command","risk":"high","inputSchema":{"type":"object"}},{"code":"flight.return_home","kind":"command","risk":"critical","inputSchema":{"type":"object"}},{"code":"stream.video.control","kind":"command","risk":"medium","inputSchema":{"type":"object"}},{"code":"dock.debug.control","kind":"command","risk":"critical","inputSchema":{"type":"object"}},{"code":"stream.telemetry.read","kind":"stream","risk":"low","outputSchema":{"type":"object"}},{"code":"stream.sensor.read","kind":"stream","risk":"low","outputSchema":{"type":"object"}},{"code":"stream.video.read","kind":"stream","risk":"low","outputSchema":{"type":"object"}},{"code":"stream.events.read","kind":"stream","risk":"low","outputSchema":{"type":"object"}}],"streams":[{"channelKey":"telemetry.primary","capabilityCode":"stream.telemetry.read","dataType":"telemetry","schema":{"type":"object"}},{"channelKey":"sensor.primary","capabilityCode":"stream.sensor.read","dataType":"sensor","schema":{"type":"object"}},{"channelKey":"video.primary","capabilityCode":"stream.video.read","dataType":"video","schema":{"type":"object"}},{"channelKey":"events.primary","capabilityCode":"stream.events.read","dataType":"events","schema":{"type":"object"}}]}'::jsonb
)
ON CONFLICT (driver_key, version) DO UPDATE
SET display_name = excluded.display_name, manifest_json = excluded.manifest_json, updated_at = now();
--> statement-breakpoint
WITH driver AS (
	SELECT id FROM driver_definitions WHERE driver_key = 'dji.cloud' AND version = '1.0.0'
), catalog(type_key, display_name, category, model, capability_profile) AS (
	VALUES
		('dji.unknown', 'Unknown DJI device', 'unknown', null, '{"state.read":{"enabled":true,"diagnosticOnly":true}}'::jsonb),
		('dji.dock2', 'DJI Dock 2', 'dock', 'Dock 2', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"dock.debug.control":{"enabled":true,"productFamily":"dock2"}}'::jsonb),
		('dji.matrice3d', 'DJI Matrice 3D', 'aircraft', 'Matrice 3D', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"flight.return_home":{"enabled":true},"stream.telemetry.read":{"enabled":true}}'::jsonb),
		('dji.matrice3td', 'DJI Matrice 3TD', 'aircraft', 'Matrice 3TD', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"flight.return_home":{"enabled":true},"stream.telemetry.read":{"enabled":true}}'::jsonb),
		('dji.matrice3d.camera', 'Matrice 3D Camera', 'camera', 'Matrice 3D Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true}}'::jsonb),
		('dji.matrice3td.camera', 'Matrice 3TD Camera', 'camera', 'Matrice 3TD Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true},"stream.sensor.read":{"enabled":true}}'::jsonb),
		('dji.matrice3.vision-assist', 'Matrice 3 Vision Assist', 'camera', 'Matrice 3 Vision Assist', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true}}'::jsonb),
		('dji.dock2.camera', 'DJI Dock 2 Camera', 'camera', 'Dock 2 Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true}}'::jsonb),
		('dji.dock2.environment-sensor', 'DJI Dock 2 Environment Sensor', 'sensor', 'Dock 2 Environment Sensor', '{"state.read":{"enabled":true},"stream.sensor.read":{"enabled":true}}'::jsonb),
		('dji.dock3', 'DJI Dock 3', 'dock', 'Dock 3', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"dock.debug.control":{"enabled":true,"productFamily":"dock3"}}'::jsonb),
		('dji.matrice4d', 'DJI Matrice 4D', 'aircraft', 'Matrice 4D', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"flight.return_home":{"enabled":true},"stream.telemetry.read":{"enabled":true}}'::jsonb),
		('dji.matrice4td', 'DJI Matrice 4TD', 'aircraft', 'Matrice 4TD', '{"state.read":{"enabled":true},"mission.execute":{"enabled":true},"mission.cancel":{"enabled":true},"flight.return_home":{"enabled":true},"stream.telemetry.read":{"enabled":true}}'::jsonb),
		('dji.matrice4d.camera', 'Matrice 4D Camera', 'camera', 'Matrice 4D Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true}}'::jsonb),
		('dji.matrice4td.camera', 'Matrice 4TD Camera', 'camera', 'Matrice 4TD Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true},"stream.sensor.read":{"enabled":true}}'::jsonb),
		('dji.matrice4.vision-assist', 'Matrice 4 Vision Assist', 'camera', 'Matrice 4 Vision Assist', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true}}'::jsonb),
		('dji.dock3.camera', 'DJI Dock 3 Camera', 'camera', 'Dock 3 Camera', '{"state.read":{"enabled":true},"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true}}'::jsonb),
		('dji.dock3.environment-sensor', 'DJI Dock 3 Environment Sensor', 'sensor', 'Dock 3 Environment Sensor', '{"state.read":{"enabled":true},"stream.sensor.read":{"enabled":true}}'::jsonb)
)
INSERT INTO device_types (
	type_key, version, display_name, category, vendor, model,
	driver_definition_id, driver_version_constraint, capability_profile_json
)
SELECT catalog.type_key, 1, catalog.display_name, catalog.category, 'dji', catalog.model,
	   driver.id, '^1.0.0', catalog.capability_profile
FROM catalog CROSS JOIN driver
ON CONFLICT (type_key, version) DO UPDATE
SET display_name = excluded.display_name, category = excluded.category, vendor = excluded.vendor,
	model = excluded.model, driver_definition_id = excluded.driver_definition_id,
	driver_version_constraint = excluded.driver_version_constraint,
	capability_profile_json = excluded.capability_profile_json, updated_at = now();
--> statement-breakpoint
UPDATE driver_definitions
SET manifest_json = jsonb_set(manifest_json, '{streams}', '[
  {"channelKey":"telemetry.primary","capabilityCode":"stream.telemetry.read","dataType":"telemetry","unit":"mixed","schema":{"type":"object","properties":{"seq":{"type":"integer"},"latitude":{"type":"number","x-unit":"degree"},"longitude":{"type":"number","x-unit":"degree"},"height":{"type":"number","x-unit":"m"},"horizontal_speed":{"type":"number","x-unit":"m/s"},"vertical_speed":{"type":"number","x-unit":"m/s"}}}},
  {"channelKey":"sensor.primary","capabilityCode":"stream.sensor.read","dataType":"sensor","unit":"mixed","schema":{"type":"object","properties":{"samples":{"type":"object","additionalProperties":{"type":"object","required":["value","unit"],"properties":{"value":{},"unit":{"type":"string"}}}}},"required":["samples"]}},
  {"channelKey":"video.primary","capabilityCode":"stream.video.read","dataType":"video","schema":{"type":"object","properties":{"sessionId":{"type":"string"},"state":{"type":"string"},"playback":{"type":"object"}}}},
  {"channelKey":"events.primary","capabilityCode":"stream.events.read","dataType":"events","schema":{"type":"object"}}
]'::jsonb, true), updated_at=now()
WHERE driver_key='dji.cloud' and version='1.0.0';
--> statement-breakpoint
ALTER TABLE "devices" ADD COLUMN "device_type_id" bigint;
--> statement-breakpoint
UPDATE "devices" SET "device_type_id" = (
	SELECT "id" FROM "device_types" WHERE "type_key" = 'legacy.device' AND "version" = 1
) WHERE "device_type_id" IS NULL;
--> statement-breakpoint
ALTER TABLE "devices" ALTER COLUMN "device_type_id" SET NOT NULL;
--> statement-breakpoint
ALTER TABLE "devices" ADD CONSTRAINT "devices_device_type_fk"
FOREIGN KEY ("device_type_id") REFERENCES "device_types"("id") ON DELETE restrict;
--> statement-breakpoint
ALTER TABLE "devices" DROP CONSTRAINT "devices_connectivity_status_valid";
--> statement-breakpoint
ALTER TABLE "devices" ADD CONSTRAINT "devices_connectivity_status_valid"
CHECK (status in ('online', 'degraded', 'offline', 'unknown', 'unavailable'));
--> statement-breakpoint
CREATE TABLE "device_relationships" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"from_device_id" integer NOT NULL,
	"to_device_id" integer NOT NULL,
	"relation_type" text NOT NULL,
	"source_type" text DEFAULT 'manual' NOT NULL,
	"valid_from" timestamp with time zone DEFAULT now() NOT NULL,
	"valid_until" timestamp with time zone,
	"metadata_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "device_relationships_project_team_fk" FOREIGN KEY ("project_id", "team_id") REFERENCES "projects"("id", "team_id") ON DELETE cascade,
	CONSTRAINT "device_relationships_from_project_fk" FOREIGN KEY ("from_device_id", "project_id") REFERENCES "devices"("id", "project_id") ON DELETE cascade,
	CONSTRAINT "device_relationships_to_project_fk" FOREIGN KEY ("to_device_id", "project_id") REFERENCES "devices"("id", "project_id") ON DELETE cascade,
	CONSTRAINT "device_relationships_not_self" CHECK (from_device_id <> to_device_id),
	CONSTRAINT "device_relationships_valid_range" CHECK (valid_until is null or valid_until > valid_from),
	CONSTRAINT "device_relationships_source_valid" CHECK (source_type in ('driver', 'discovery', 'manual', 'migration')),
	CONSTRAINT "device_relationships_unique" UNIQUE("project_id", "from_device_id", "to_device_id", "relation_type", "valid_from"),
	CONSTRAINT "device_relationships_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
ALTER TABLE "device_capabilities"
	ADD COLUMN "device_type_id" bigint,
	ADD COLUMN "driver_definition_id" bigint,
	ADD COLUMN "availability" text DEFAULT 'available' NOT NULL,
	ADD COLUMN "availability_reason" text,
	ADD COLUMN "input_schema_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	ADD COLUMN "output_schema_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	ADD COLUMN "risk_level" text DEFAULT 'low' NOT NULL,
	ADD COLUMN "source_json" jsonb DEFAULT '{}'::jsonb NOT NULL;
--> statement-breakpoint
UPDATE "device_capabilities" capability
SET "device_type_id" = device."device_type_id"
FROM "devices" device WHERE device."id" = capability."device_id";
--> statement-breakpoint
UPDATE "device_capabilities" capability
SET "driver_definition_id" = device_type."driver_definition_id"
FROM "device_types" device_type WHERE device_type."id" = capability."device_type_id";
--> statement-breakpoint
CREATE OR REPLACE FUNCTION populate_device_capability_type_driver()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF NEW.device_type_id IS NULL OR NEW.driver_definition_id IS NULL THEN
		SELECT device.device_type_id, device_type.driver_definition_id
		INTO NEW.device_type_id, NEW.driver_definition_id
		FROM devices device
		JOIN device_types device_type ON device_type.id = device.device_type_id
		WHERE device.id = NEW.device_id AND device.project_id = NEW.project_id;
	END IF;
	RETURN NEW;
END;
$$;
--> statement-breakpoint
CREATE TRIGGER device_capabilities_populate_type_driver
BEFORE INSERT OR UPDATE OF device_id, project_id, device_type_id, driver_definition_id
ON device_capabilities
FOR EACH ROW EXECUTE FUNCTION populate_device_capability_type_driver();
--> statement-breakpoint
ALTER TABLE "device_capabilities"
	ALTER COLUMN "device_type_id" SET NOT NULL,
	ALTER COLUMN "driver_definition_id" SET NOT NULL,
	ADD CONSTRAINT "device_capabilities_device_type_fk" FOREIGN KEY ("device_type_id") REFERENCES "device_types"("id") ON DELETE restrict,
	ADD CONSTRAINT "device_capabilities_driver_fk" FOREIGN KEY ("driver_definition_id") REFERENCES "driver_definitions"("id") ON DELETE restrict,
	ADD CONSTRAINT "device_capabilities_availability_valid" CHECK (availability in ('available', 'degraded', 'unavailable')),
	ADD CONSTRAINT "device_capabilities_risk_valid" CHECK (risk_level in ('low', 'medium', 'high', 'critical'));
--> statement-breakpoint
CREATE TABLE "device_network_profiles" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"name" text NOT NULL,
	"mode" text NOT NULL,
	"mqtt_endpoint" text,
	"api_public_base_url" text,
	"websocket_public_url" text,
	"media_ingest_base_url" text,
	"media_playback_base_url" text,
	"tls_required" boolean DEFAULT false NOT NULL,
	"secret_ref" text,
	"status" text DEFAULT 'unverified' NOT NULL,
	"config_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"last_validation_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"last_validated_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "device_network_profiles_project_team_fk" FOREIGN KEY ("project_id", "team_id") REFERENCES "projects"("id", "team_id") ON DELETE cascade,
	CONSTRAINT "device_network_profiles_mode_valid" CHECK (mode in ('lan', 'public')),
	CONSTRAINT "device_network_profiles_status_valid" CHECK (status in ('unverified', 'valid', 'invalid', 'degraded')),
	CONSTRAINT "device_network_profiles_public_tls" CHECK (mode <> 'public' or tls_required),
	CONSTRAINT "device_network_profiles_project_name_unique" UNIQUE("project_id", "name"),
	CONSTRAINT "device_network_profiles_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
ALTER TABLE "device_adapters" ADD COLUMN "network_profile_id" bigint;
--> statement-breakpoint
ALTER TABLE "device_adapters" ADD CONSTRAINT "device_adapters_network_profile_project_fk"
FOREIGN KEY ("network_profile_id", "project_id") REFERENCES "device_network_profiles"("id", "project_id") ON DELETE SET NULL ("network_profile_id");
--> statement-breakpoint
CREATE TABLE "device_stream_channels" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"device_id" integer NOT NULL,
	"stable_channel_id" text NOT NULL,
	"capability_code" text NOT NULL,
	"channel_key" text NOT NULL,
	"display_name" text NOT NULL,
	"data_type" text NOT NULL,
	"schema_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"unit" text,
	"protocol" text,
	"quality_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"availability" text DEFAULT 'available' NOT NULL,
	"availability_reason" text,
	"source_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "device_stream_channels_project_team_fk" FOREIGN KEY ("project_id", "team_id") REFERENCES "projects"("id", "team_id") ON DELETE cascade,
	CONSTRAINT "device_stream_channels_device_project_fk" FOREIGN KEY ("device_id", "project_id") REFERENCES "devices"("id", "project_id") ON DELETE cascade,
	CONSTRAINT "device_stream_channels_capability_fk" FOREIGN KEY ("device_id", "capability_code") REFERENCES "device_capabilities"("device_id", "capability_code") ON DELETE cascade,
	CONSTRAINT "device_stream_channels_data_type_valid" CHECK (data_type in ('video', 'audio', 'telemetry', 'sensor', 'events')),
	CONSTRAINT "device_stream_channels_availability_valid" CHECK (availability in ('available', 'degraded', 'unavailable')),
	CONSTRAINT "device_stream_channels_device_key_unique" UNIQUE("device_id", "channel_key"),
	CONSTRAINT "device_stream_channels_project_stable_unique" UNIQUE("project_id", "stable_channel_id"),
	CONSTRAINT "device_stream_channels_id_project_unique" UNIQUE("id", "project_id")
);
--> statement-breakpoint
CREATE OR REPLACE FUNCTION populate_device_stream_channel_stable_id()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF NEW.stable_channel_id IS NULL OR length(trim(NEW.stable_channel_id)) = 0 THEN
		NEW.stable_channel_id := 'device:' || NEW.project_id || ':' || NEW.device_id || ':' || NEW.channel_key;
	END IF;
	RETURN NEW;
END;
$$;
--> statement-breakpoint
CREATE TRIGGER device_stream_channels_populate_stable_id
BEFORE INSERT OR UPDATE OF project_id, device_id, channel_key, stable_channel_id
ON device_stream_channels FOR EACH ROW EXECUTE FUNCTION populate_device_stream_channel_stable_id();
--> statement-breakpoint
CREATE TABLE "device_capability_grants" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"project_id" integer NOT NULL,
	"team_id" integer NOT NULL,
	"user_id" integer NOT NULL,
	"scope_type" text NOT NULL,
	"device_type_id" bigint,
	"device_id" integer,
	"action_pattern" text NOT NULL,
	"effect" text DEFAULT 'allow' NOT NULL,
	"granted_by_user_id" integer,
	"expires_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "device_capability_grants_project_team_fk" FOREIGN KEY ("project_id", "team_id") REFERENCES "projects"("id", "team_id") ON DELETE cascade,
	CONSTRAINT "device_capability_grants_team_member_fk" FOREIGN KEY ("team_id", "user_id") REFERENCES "team_members"("team_id", "user_id") ON DELETE cascade,
	CONSTRAINT "device_capability_grants_type_fk" FOREIGN KEY ("device_type_id") REFERENCES "device_types"("id") ON DELETE cascade,
	CONSTRAINT "device_capability_grants_device_project_fk" FOREIGN KEY ("device_id", "project_id") REFERENCES "devices"("id", "project_id") ON DELETE cascade,
	CONSTRAINT "device_capability_grants_granter_fk" FOREIGN KEY ("granted_by_user_id") REFERENCES "users"("id") ON DELETE set null,
	CONSTRAINT "device_capability_grants_scope_valid" CHECK ((scope_type = 'project' and device_type_id is null and device_id is null) or (scope_type = 'device_type' and device_type_id is not null and device_id is null) or (scope_type = 'device' and device_type_id is null and device_id is not null)),
	CONSTRAINT "device_capability_grants_effect_valid" CHECK (effect in ('allow', 'deny')),
	CONSTRAINT "device_capability_grants_action_nonempty" CHECK (length(trim(action_pattern)) > 0)
);
--> statement-breakpoint
ALTER TABLE "live_streams"
	ADD COLUMN "stream_channel_id" bigint,
	ADD COLUMN "ingest_ref" text,
	ADD COLUMN "lease_expires_at" timestamp with time zone,
	ADD COLUMN "lease_owner" text,
	ADD CONSTRAINT "live_streams_lease_complete" CHECK (
		("lease_owner" is null and "lease_expires_at" is null)
		or ("lease_owner" is not null and "lease_expires_at" is not null)
	);
--> statement-breakpoint
CREATE UNIQUE INDEX "live_streams_ingest_ref_unique" ON "live_streams" ("ingest_ref") WHERE "ingest_ref" is not null;
--> statement-breakpoint
CREATE INDEX "live_streams_expired_lease_idx" ON "live_streams" ("lease_expires_at") WHERE "status" in ('requested', 'starting', 'live', 'degraded', 'stopping');
--> statement-breakpoint
ALTER TABLE "device_commands" ADD COLUMN "live_stream_id" bigint;
--> statement-breakpoint
ALTER TABLE "device_commands" ADD CONSTRAINT "device_commands_live_stream_project_fk"
FOREIGN KEY ("live_stream_id", "project_id") REFERENCES "live_streams"("id", "project_id") ON DELETE SET NULL ("live_stream_id");
--> statement-breakpoint
CREATE UNIQUE INDEX "device_commands_live_stream_action_unique" ON "device_commands" ("live_stream_id", "command_key") WHERE "live_stream_id" is not null;
--> statement-breakpoint
ALTER TABLE "live_streams" ADD CONSTRAINT "live_streams_channel_project_fk"
FOREIGN KEY ("stream_channel_id", "project_id") REFERENCES "device_stream_channels"("id", "project_id") ON DELETE SET NULL ("stream_channel_id");
--> statement-breakpoint
ALTER TABLE "algorithm_definition_versions"
	ADD COLUMN "output_schema_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	ADD COLUMN "display_metadata_json" jsonb DEFAULT '{}'::jsonb NOT NULL;
--> statement-breakpoint
CREATE INDEX "devices_project_type_idx" ON "devices" ("project_id", "device_type_id");
--> statement-breakpoint
CREATE INDEX "device_relationships_from_idx" ON "device_relationships" ("from_device_id", "relation_type", "valid_from" DESC);
--> statement-breakpoint
CREATE INDEX "device_relationships_to_idx" ON "device_relationships" ("to_device_id", "relation_type", "valid_from" DESC);
--> statement-breakpoint
CREATE INDEX "device_capabilities_type_code_idx" ON "device_capabilities" ("device_type_id", "capability_code");
--> statement-breakpoint
CREATE INDEX "device_stream_channels_project_type_idx" ON "device_stream_channels" ("project_id", "data_type", "availability");
--> statement-breakpoint
CREATE UNIQUE INDEX "device_capability_grants_unique" ON "device_capability_grants" ("project_id", "user_id", "scope_type", "device_type_id", "device_id", "action_pattern") NULLS NOT DISTINCT;
--> statement-breakpoint
CREATE INDEX "device_capability_grants_lookup_idx" ON "device_capability_grants" ("project_id", "user_id", "action_pattern", "expires_at");
--> statement-breakpoint
CREATE INDEX "driver_definitions_status_idx" ON "driver_definitions" ("status", "driver_key");
--> statement-breakpoint
CREATE INDEX "device_types_driver_idx" ON "device_types" ("driver_definition_id", "status", "type_key");
--> statement-breakpoint
ALTER TABLE "devices"
	ADD COLUMN "status_observed_at" timestamp with time zone,
	ADD COLUMN "status_projected_at" timestamp with time zone DEFAULT now() NOT NULL,
	ADD COLUMN "data_freshness" text DEFAULT 'unknown' NOT NULL,
	ADD COLUMN "raw_status_ref" text;
--> statement-breakpoint
ALTER TABLE "devices" ADD CONSTRAINT "devices_data_freshness_valid"
CHECK ("data_freshness" in ('fresh', 'stale', 'expired', 'unknown'));
--> statement-breakpoint
CREATE OR REPLACE FUNCTION protect_published_algorithm_definition_version()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF TG_OP = 'DELETE' AND OLD.status IN ('published','retired') THEN
		RAISE EXCEPTION 'published algorithm definition versions are immutable' USING ERRCODE = '55000';
	END IF;
	IF TG_OP = 'UPDATE' AND OLD.status = 'published' THEN
		IF NEW.status = 'retired'
		   AND NEW.execution_mode = OLD.execution_mode
		   AND NEW.model_or_process = OLD.model_or_process
		   AND NEW.input_requirements_json = OLD.input_requirements_json
		   AND NEW.parameters_schema_json = OLD.parameters_schema_json
		   AND NEW.protocol_config_json = OLD.protocol_config_json
		   AND NEW.output_mapping_json = OLD.output_mapping_json
		   AND NEW.label_mapping_json = OLD.label_mapping_json
		   AND NEW.output_schema_json = OLD.output_schema_json
		   AND NEW.display_metadata_json = OLD.display_metadata_json
		   AND NEW.publish_threshold = OLD.publish_threshold THEN
			RETURN NEW;
		END IF;
		RAISE EXCEPTION 'published algorithm definition versions are immutable' USING ERRCODE = '55000';
	END IF;
	IF TG_OP = 'UPDATE' AND OLD.status = 'retired' THEN
		RAISE EXCEPTION 'published algorithm definition versions are immutable' USING ERRCODE = '55000';
	END IF;
	RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;
--> statement-breakpoint
create table connector_definitions (
  id bigserial primary key,
  connector_key text not null,
  version text not null,
  display_name text not null,
  status text not null default 'active',
  manifest_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_definitions_key_version_unique unique (connector_key, version),
  constraint connector_definitions_status_valid check (status in ('active', 'disabled', 'retired')),
  constraint connector_definitions_manifest_object check (jsonb_typeof(manifest_json) = 'object')
);
--> statement-breakpoint
insert into connector_definitions (connector_key, version, display_name, manifest_json)
values
  ('dji.cloud-api', '1.0.0', 'DJI Cloud API',
   '{"discoveryModes":["subscribe","push"],"protocols":["mqtt","https","websocket"],"compatibleDrivers":["dji.cloud"]}'::jsonb),
  ('simulator.memory', '1.0.0', 'In-memory Simulator',
   '{"discoveryModes":["push"],"protocols":["memory"],"compatibleDrivers":["simulator"]}'::jsonb),
  ('legacy.adapter', '1.0.0', 'Legacy Adapter Compatibility',
   '{"discoveryModes":["manual-import"],"protocols":[],"compatibleDrivers":["*"]}'::jsonb)
on conflict (connector_key, version) do nothing;
--> statement-breakpoint
alter table device_adapters
  add column connector_definition_id bigint,
  add column onboarding_policy text not null default 'review',
  add column discovery_scope_json jsonb not null default '{}'::jsonb,
  add column sync_cursor_json jsonb not null default '{}'::jsonb;
--> statement-breakpoint
update device_adapters adapter
   set connector_definition_id = definition.id
  from connector_definitions definition
 where definition.version = '1.0.0'
   and definition.connector_key = case adapter.adapter_type
     when 'dji' then 'dji.cloud-api'
     when 'simulator' then 'simulator.memory'
     else 'legacy.adapter'
   end;
--> statement-breakpoint
alter table device_adapters
  alter column connector_definition_id set not null,
  add constraint device_adapters_connector_definition_fk
    foreign key (connector_definition_id) references connector_definitions(id) on delete restrict,
  add constraint device_adapters_onboarding_policy_valid
    check (onboarding_policy in ('automatic', 'review', 'observe-only')),
  add constraint device_adapters_discovery_scope_object
    check (jsonb_typeof(discovery_scope_json) = 'object'),
  add constraint device_adapters_sync_cursor_object
    check (jsonb_typeof(sync_cursor_json) = 'object');
--> statement-breakpoint
create or replace function populate_device_adapter_connector_definition()
returns trigger language plpgsql as $$
begin
  if new.connector_definition_id is null then
    select definition.id into new.connector_definition_id
      from connector_definitions definition
     where definition.version = '1.0.0'
       and definition.connector_key = case new.adapter_type
         when 'dji' then 'dji.cloud-api'
         when 'simulator' then 'simulator.memory'
         else 'legacy.adapter'
       end;
  end if;
  return new;
end;
$$;
--> statement-breakpoint
create trigger device_adapters_populate_connector_definition
before insert or update of adapter_type, connector_definition_id
on device_adapters
for each row execute function populate_device_adapter_connector_definition();
--> statement-breakpoint
create index device_adapters_connector_definition_idx
  on device_adapters(connector_definition_id, status);
--> statement-breakpoint
create view connector_instances as
select adapter.id,
       adapter.project_id,
       adapter.team_id,
       adapter.name,
       adapter.connector_definition_id,
       definition.connector_key,
       definition.version as connector_version,
       adapter.adapter_type as legacy_adapter_type,
       adapter.vendor,
       adapter.protocol_version,
       adapter.status,
       adapter.secret_ref,
       adapter.config_json,
       adapter.capabilities_json,
       adapter.network_profile_id,
       adapter.onboarding_policy,
       adapter.discovery_scope_json,
       adapter.sync_cursor_json,
       adapter.last_health_json,
       adapter.last_checked_at,
       adapter.created_at,
       adapter.updated_at
  from device_adapters adapter
  join connector_definitions definition on definition.id = adapter.connector_definition_id;
--> statement-breakpoint
create table connector_sync_runs (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  discovery_mode text not null,
  status text not null default 'pending',
  scope_json jsonb not null default '{}'::jsonb,
  cursor_before_json jsonb not null default '{}'::jsonb,
  cursor_after_json jsonb not null default '{}'::jsonb,
  discovered_count integer not null default 0,
  managed_count integer not null default 0,
  missing_count integer not null default 0,
  error_code text,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  constraint connector_sync_runs_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint connector_sync_runs_connector_project_fk
    foreign key (connector_instance_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint connector_sync_runs_mode_valid
    check (discovery_mode in ('push', 'poll', 'subscribe', 'manual-import')),
  constraint connector_sync_runs_status_valid
    check (status in ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
  constraint connector_sync_runs_counts_nonnegative
    check (discovered_count >= 0 and managed_count >= 0 and missing_count >= 0),
  constraint connector_sync_runs_time_valid
    check (finished_at is null or (started_at is not null and finished_at >= started_at)),
  constraint connector_sync_runs_scope_object check (jsonb_typeof(scope_json) = 'object'),
  constraint connector_sync_runs_cursor_before_object check (jsonb_typeof(cursor_before_json) = 'object'),
  constraint connector_sync_runs_cursor_after_object check (jsonb_typeof(cursor_after_json) = 'object'),
  constraint connector_sync_runs_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create index connector_sync_runs_connector_created_idx
  on connector_sync_runs(connector_instance_id, created_at desc);
--> statement-breakpoint
alter table device_external_identities
  add column discovery_status text not null default 'discovered',
  add column suggested_device_type_id bigint,
  add column match_confidence double precision,
  add column source_version text,
  add column last_sync_run_id bigint;
--> statement-breakpoint
update device_external_identities
   set discovery_status = case when device_id is null then 'discovered' else 'managed' end;
--> statement-breakpoint
alter table device_external_identities
  add constraint device_external_identities_status_valid
    check (discovery_status in ('discovered', 'managed', 'ignored', 'conflicted', 'missing')),
  add constraint device_external_identities_match_confidence_valid
    check (match_confidence is null or (match_confidence >= 0 and match_confidence <= 1)),
  add constraint device_external_identities_suggested_type_fk
    foreign key (suggested_device_type_id) references device_types(id) on delete set null,
  add constraint device_external_identities_sync_run_project_fk
    foreign key (last_sync_run_id, project_id)
    references connector_sync_runs(id, project_id) on delete set null (last_sync_run_id),
  add constraint device_external_identities_connector_identity_project_unique
    unique (id, adapter_id, project_id);
--> statement-breakpoint
insert into device_external_identities (
  project_id, team_id, adapter_id, device_id, external_device_id,
  external_device_type, identity_json, discovery_status, bound_at
)
select device.project_id, project.team_id, device.adapter_id, device.id,
       'migration:device:' || device.id::text, device.type,
       jsonb_build_object('source', 'connector-migration', 'legacyDeviceId', device.id),
       'managed', now()
  from devices device
  join projects project on project.id = device.project_id
 where device.adapter_id is not null
   and not exists (
     select 1 from device_external_identities identity
      where identity.project_id = device.project_id
        and identity.adapter_id = device.adapter_id
        and identity.device_id = device.id
   );
--> statement-breakpoint
create table device_connector_bindings (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  device_id integer not null,
  connector_instance_id bigint not null,
  external_identity_id bigint not null,
  route_role text not null default 'direct',
  priority integer not null default 100,
  status text not null default 'active',
  bound_at timestamptz not null default now(),
  unbound_at timestamptz,
  metadata_json jsonb not null default '{}'::jsonb,
  constraint device_connector_bindings_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_connector_bindings_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint device_connector_bindings_connector_project_fk
    foreign key (connector_instance_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint device_connector_bindings_identity_connector_project_fk
    foreign key (external_identity_id, connector_instance_id, project_id)
    references device_external_identities(id, adapter_id, project_id) on delete cascade,
  constraint device_connector_bindings_route_role_valid
    check (route_role in ('direct', 'gateway', 'inherited')),
  constraint device_connector_bindings_priority_nonnegative check (priority >= 0),
  constraint device_connector_bindings_status_valid
    check (status in ('active', 'standby', 'disabled', 'conflicted')),
  constraint device_connector_bindings_time_valid
    check (unbound_at is null or unbound_at >= bound_at),
  constraint device_connector_bindings_metadata_object check (jsonb_typeof(metadata_json) = 'object'),
  constraint device_connector_bindings_identity_unique unique (external_identity_id),
  constraint device_connector_bindings_device_connector_unique unique (device_id, connector_instance_id),
  constraint device_connector_bindings_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
insert into device_connector_bindings (
  project_id, team_id, device_id, connector_instance_id, external_identity_id,
  route_role, priority, status, bound_at, metadata_json
)
select identity.project_id, identity.team_id, identity.device_id, identity.adapter_id, identity.id,
       'direct', 100, 'active', coalesce(identity.bound_at, now()),
       jsonb_build_object('source', 'connector-migration')
  from device_external_identities identity
 where identity.device_id is not null
on conflict (device_id, connector_instance_id) do nothing;
--> statement-breakpoint
create index device_connector_bindings_device_route_idx
  on device_connector_bindings(device_id, status, priority);
--> statement-breakpoint
create index device_connector_bindings_connector_idx
  on device_connector_bindings(connector_instance_id, status);
--> statement-breakpoint
create index device_external_identities_discovery_idx
  on device_external_identities(project_id, discovery_status, last_seen_at desc);
--> statement-breakpoint
alter table device_adapters
  add column credential_envelope_json jsonb,
  add constraint device_adapters_credential_envelope_object
    check (credential_envelope_json is null or jsonb_typeof(credential_envelope_json) = 'object');
--> statement-breakpoint
alter table algorithm_providers
  add column credential_envelope_json jsonb,
  add constraint algorithm_providers_credential_envelope_object
    check (credential_envelope_json is null or jsonb_typeof(credential_envelope_json) = 'object');
--> statement-breakpoint
create table ai_providers (
  id bigserial primary key,
  name text not null,
  provider_type text not null,
  base_url text,
  model_id text not null,
  credential_envelope_json jsonb not null,
  enabled boolean not null default false,
  is_default boolean not null default false,
  status text not null default 'untested',
  health_json jsonb not null default '{}'::jsonb,
  last_tested_at timestamptz,
  created_by_user_id integer not null references users(id) on delete restrict,
  updated_by_user_id integer not null references users(id) on delete restrict,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint ai_providers_name_unique unique (name),
  constraint ai_providers_type_valid check (provider_type in ('openai')),
  constraint ai_providers_status_valid check (status in ('untested','healthy','degraded','failed')),
  constraint ai_providers_default_enabled check (not is_default or enabled),
  constraint ai_providers_credential_envelope_object check (jsonb_typeof(credential_envelope_json) = 'object')
);
--> statement-breakpoint
create unique index ai_providers_single_default_idx on ai_providers(is_default) where is_default;
--> statement-breakpoint
create table platform_audit_events (
  id bigserial primary key,
  actor_user_id integer not null references users(id) on delete restrict,
  request_id text not null,
  action text not null,
  resource_type text not null,
  resource_id text,
  input_hash text not null,
  result_hash text,
  status text not null default 'accepted',
  created_at timestamptz not null default now(),
  completed_at timestamptz,
  constraint platform_audit_events_status_valid check (status in ('accepted','completed'))
);
--> statement-breakpoint
create index platform_audit_events_actor_created_idx on platform_audit_events(actor_user_id, created_at desc);
--> statement-breakpoint
update device_adapters
   set status = 'disabled',
       last_health_json = jsonb_build_object('ok', false, 'code', 'CREDENTIAL_REENTRY_REQUIRED'),
       updated_at = now()
 where secret_ref is not null and credential_envelope_json is null;
--> statement-breakpoint
update algorithm_providers
   set status = 'disabled',
       health_json = jsonb_build_object('ok', false, 'code', 'CREDENTIAL_REENTRY_REQUIRED'),
       updated_at = now()
 where secret_ref is not null and credential_envelope_json is null;
--> statement-breakpoint
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
    "configSchema":{"type":"object","additionalProperties":false,"properties":{}},
    "credentialSchema":{"type":"object","additionalProperties":false,"required":["token"],"properties":{"token":{"type":"string","minLength":1,"writeOnly":true}}},
    "discoveryScopeSchema":{"type":"object","additionalProperties":false,"required":["projectUuid","projectName"],"properties":{"projectUuid":{"type":"string","format":"uuid"},"projectName":{"type":"string","minLength":1}}}
  }'::jsonb
)
on conflict (connector_key, version) do nothing;
--> statement-breakpoint
alter table device_adapters
  add column external_scope_key text,
  add constraint device_adapters_external_scope_key_normalized
    check (
      external_scope_key is null
      or (
        length(external_scope_key) between 1 and 512
        and external_scope_key = btrim(external_scope_key)
      )
    );
--> statement-breakpoint
create unique index device_adapters_connector_external_scope_unique
  on device_adapters(project_id, connector_definition_id, external_scope_key)
  where external_scope_key is not null;
--> statement-breakpoint
create or replace view connector_instances as
select adapter.id,
       adapter.project_id,
       adapter.team_id,
       adapter.name,
       adapter.connector_definition_id,
       definition.connector_key,
       definition.version as connector_version,
       adapter.adapter_type as legacy_adapter_type,
       adapter.vendor,
       adapter.protocol_version,
       adapter.status,
       adapter.secret_ref,
       adapter.config_json,
       adapter.capabilities_json,
       adapter.network_profile_id,
       adapter.onboarding_policy,
       adapter.discovery_scope_json,
       adapter.sync_cursor_json,
       adapter.last_health_json,
       adapter.last_checked_at,
       adapter.created_at,
       adapter.updated_at,
       adapter.external_scope_key,
       adapter.credential_envelope_json
  from device_adapters adapter
  join connector_definitions definition on definition.id = adapter.connector_definition_id;
--> statement-breakpoint
alter table task_runs
  add column trigger_key text,
  add constraint task_runs_trigger_key_normalized
    check (trigger_key is null or (length(trigger_key) between 3 and 512 and trigger_key = btrim(trigger_key)));
--> statement-breakpoint
create unique index task_runs_trigger_key_unique
  on task_runs(project_id, task_version_id, trigger_key)
  where task_version_id is not null and trigger_key is not null;
--> statement-breakpoint
create index task_runs_active_concurrency_idx
  on task_runs(project_id, task_version_id, status)
  where status in ('queued','blocked','ready','dispatching','running','paused','canceling');
--> statement-breakpoint
create table issue_feedback (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  issue_id integer not null,
  detection_id bigint not null,
  algorithm_definition_version_id bigint not null,
  task_version_id bigint,
  task_run_step_id bigint,
  action text not null,
  corrected_label text,
  disposition text,
  reason text not null,
  client_key uuid not null,
  evidence_snapshot_json jsonb not null,
  actor_user_id integer not null references users(id) on delete restrict,
  created_at timestamptz not null default now(),
  constraint issue_feedback_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint issue_feedback_issue_project_fk foreign key(issue_id,project_id) references issues(id,project_id) on delete cascade,
  constraint issue_feedback_detection_project_fk foreign key(detection_id,project_id) references detections(id,project_id) on delete restrict,
  constraint issue_feedback_algorithm_version_project_fk foreign key(algorithm_definition_version_id,project_id) references algorithm_definition_versions(id,project_id) on delete restrict,
  constraint issue_feedback_task_version_project_fk foreign key(task_version_id,project_id) references task_versions(id,project_id) on delete restrict,
  constraint issue_feedback_task_step_project_fk foreign key(task_run_step_id,project_id) references task_run_steps(id,project_id) on delete restrict,
  constraint issue_feedback_action_valid check(action in('confirm','false_positive','category_correction','disposition')),
  constraint issue_feedback_correction_valid check((action='category_correction')=(corrected_label is not null)),
  constraint issue_feedback_disposition_valid check(disposition is null or disposition in('resolved','monitoring','remediated','accepted_risk','not_applicable')),
  constraint issue_feedback_project_client_unique unique(project_id,client_key),
  constraint issue_feedback_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create index issue_feedback_quality_idx on issue_feedback(project_id,algorithm_definition_version_id,task_version_id,action,created_at desc);
--> statement-breakpoint
create table connector_resource_sync_states (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  resource_kind text not null,
  status text not null default 'idle',
  cursor_json jsonb not null default '{}'::jsonb,
  attempt_count integer not null default 0,
  last_error_code text,
  last_started_at timestamptz,
  last_succeeded_at timestamptz,
  next_attempt_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_resource_sync_states_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint connector_resource_sync_states_connector_project_fk
    foreign key (connector_instance_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint connector_resource_sync_states_kind_valid
    check (resource_kind in (
      'inventory','device-state','health','active-operations','waylines',
      'flight-tasks','flight-artifacts','live','geospatial','models','organization'
    )),
  constraint connector_resource_sync_states_status_valid
    check (status in ('idle','running','backoff','failed','disabled')),
  constraint connector_resource_sync_states_attempt_nonnegative check (attempt_count >= 0),
  constraint connector_resource_sync_states_cursor_object check (jsonb_typeof(cursor_json) = 'object'),
  constraint connector_resource_sync_states_time_valid
    check (last_succeeded_at is null or last_started_at is null or last_succeeded_at >= last_started_at),
  constraint connector_resource_sync_states_unique
    unique (project_id, connector_instance_id, resource_kind),
  constraint connector_resource_sync_states_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create index connector_resource_sync_states_due_idx
  on connector_resource_sync_states(connector_instance_id, status, next_attempt_at);
--> statement-breakpoint
create table connector_remote_resources (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  resource_kind text not null,
  remote_id text not null,
  remote_version text,
  remote_updated_at timestamptz,
  status text not null default 'active',
  summary_json jsonb not null default '{}'::jsonb,
  canonical_target_type text,
  canonical_target_id text,
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  missing_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_remote_resources_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint connector_remote_resources_connector_project_fk
    foreign key (connector_instance_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint connector_remote_resources_kind_valid
    check (resource_kind in (
      'wayline','flight-task','flight-media','flight-record','flight-alert','ai-alert',
      'map-element','flight-area','offline-map','air-sense-warning','model','model-resource',
      'live-share','stream-converter','recording','hms','topology','auto-record',
      'organization-user','organization-role','organization-permission'
    )),
  constraint connector_remote_resources_status_valid
    check (status in ('active','missing','deleted','failed')),
  constraint connector_remote_resources_remote_id_valid
    check (length(btrim(remote_id)) between 1 and 512 and remote_id = btrim(remote_id)),
  constraint connector_remote_resources_summary_object check (jsonb_typeof(summary_json) = 'object'),
  constraint connector_remote_resources_canonical_pair
    check ((canonical_target_type is null) = (canonical_target_id is null)),
  constraint connector_remote_resources_missing_time
    check ((status = 'missing') = (missing_at is not null) or status in ('deleted','failed')),
  constraint connector_remote_resources_unique
    unique (project_id, connector_instance_id, resource_kind, remote_id),
  constraint connector_remote_resources_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create index connector_remote_resources_lookup_idx
  on connector_remote_resources(project_id, resource_kind, status, last_seen_at desc);
--> statement-breakpoint
create index connector_remote_resources_canonical_idx
  on connector_remote_resources(project_id, canonical_target_type, canonical_target_id)
  where canonical_target_type is not null;
--> statement-breakpoint
create table connector_capability_snapshots (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  capability_code text not null,
  status text not null,
  evidence_level text not null,
  region text not null,
  deployment text not null,
  device_model text,
  firmware_version text,
  details_json jsonb not null default '{}'::jsonb,
  verified_at timestamptz not null,
  expires_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_capability_snapshots_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint connector_capability_snapshots_connector_project_fk
    foreign key (connector_instance_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint connector_capability_snapshots_status_valid
    check (status in ('supported','empty','forbidden','not_applicable','unverified','degraded','failed')),
  constraint connector_capability_snapshots_evidence_valid
    check (evidence_level in ('documented','fixture','live-read','field-write')),
  constraint connector_capability_snapshots_identity_valid
    check (
      length(btrim(capability_code)) between 1 and 256
      and capability_code = btrim(capability_code)
      and length(btrim(region)) between 1 and 64
      and length(btrim(deployment)) between 1 and 128
    ),
  constraint connector_capability_snapshots_details_object check (jsonb_typeof(details_json) = 'object'),
  constraint connector_capability_snapshots_time_valid check (expires_at is null or expires_at > verified_at),
  constraint connector_capability_snapshots_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create unique index connector_capability_snapshots_identity_unique
  on connector_capability_snapshots(
    project_id, connector_instance_id, capability_code, region, deployment, device_model, firmware_version
  ) nulls not distinct;
--> statement-breakpoint
create index connector_capability_snapshots_effective_idx
  on connector_capability_snapshots(connector_instance_id, status, expires_at);
--> statement-breakpoint
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
--> statement-breakpoint
alter table observations
  add column task_run_id integer,
  add constraint observations_task_run_project_fk
    foreign key(task_run_id,project_id) references task_runs(id,project_id)
    on delete set null (task_run_id);
--> statement-breakpoint
create index observations_task_run_time_idx
  on observations(project_id,task_run_id,captured_at)
  where task_run_id is not null;
--> statement-breakpoint
create table connector_asset_access_refs (
  id integer primary key,
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  remote_resource_id bigint not null,
  access_kind text not null,
  reference_digest text not null,
  credential_envelope_json jsonb not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_asset_access_refs_project_team_fk
    foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_asset_access_refs_connector_project_fk
    foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_asset_access_refs_resource_project_fk
    foreign key(remote_resource_id,project_id) references connector_remote_resources(id,project_id) on delete cascade,
  constraint connector_asset_access_refs_asset_project_fk
    foreign key(id,project_id) references assets(id,project_id) on delete cascade,
  constraint connector_asset_access_refs_kind_valid
    check(access_kind in('flight-media','flight-record')),
  constraint connector_asset_access_refs_digest_valid
    check(reference_digest ~ '^[a-f0-9]{64}$'),
  constraint connector_asset_access_refs_envelope_object
    check(jsonb_typeof(credential_envelope_json)='object'),
  constraint connector_asset_access_refs_id_project_unique unique(id,project_id),
  constraint connector_asset_access_refs_reference_unique
    unique(project_id,connector_instance_id,access_kind,reference_digest)
);
--> statement-breakpoint
create index connector_asset_access_refs_resource_idx
  on connector_asset_access_refs(project_id,connector_instance_id,remote_resource_id);
--> statement-breakpoint
create table connector_object_upload_jobs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  operation_kind text not null,
  source_asset_id integer not null,
  requested_by_user_id integer not null,
  idempotency_key text not null,
  requested_name text not null,
  reconciliation_name text not null,
  status text not null default 'queued',
  object_key_digest text,
  object_key_envelope_json jsonb,
  notification_attempt_count integer not null default 0,
  reconciliation_miss_count integer not null default 0,
  last_error_code text,
  remote_resource_id bigint,
  uploaded_at timestamptz,
  notification_attempted_at timestamptz,
  reconciled_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_object_upload_jobs_project_team_fk
    foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_object_upload_jobs_connector_project_fk
    foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_object_upload_jobs_asset_project_fk
    foreign key(source_asset_id,project_id) references assets(id,project_id) on delete restrict,
  constraint connector_object_upload_jobs_requester_member_fk
    foreign key(team_id,requested_by_user_id) references team_members(team_id,user_id) on delete restrict,
  constraint connector_object_upload_jobs_remote_project_fk
    foreign key(remote_resource_id,project_id) references connector_remote_resources(id,project_id) on delete set null (remote_resource_id),
  constraint connector_object_upload_jobs_status_valid
    check(status in('queued','uploading','notifying','reconciling','succeeded','failed')),
  constraint connector_object_upload_jobs_operation_kind_valid
    check(operation_kind ~ '^[a-z][a-z0-9-]{0,63}$'),
  constraint connector_object_upload_jobs_idempotency_valid
    check(length(btrim(idempotency_key)) between 8 and 200 and idempotency_key=btrim(idempotency_key)),
  constraint connector_object_upload_jobs_name_valid
    check(length(btrim(requested_name)) between 1 and 200 and requested_name=btrim(requested_name)
      and length(btrim(reconciliation_name)) between 1 and 240 and reconciliation_name=btrim(reconciliation_name)),
  constraint connector_object_upload_jobs_digest_valid
    check(object_key_digest is null or object_key_digest ~ '^[a-f0-9]{64}$'),
  constraint connector_object_upload_jobs_envelope_object
    check(object_key_envelope_json is null or jsonb_typeof(object_key_envelope_json)='object'),
  constraint connector_object_upload_jobs_attempts_valid
    check(notification_attempt_count between 0 and 2 and reconciliation_miss_count between 0 and 8),
  constraint connector_object_upload_jobs_upload_checkpoint
    check((object_key_digest is null)=(object_key_envelope_json is null)
      and (uploaded_at is null)=(object_key_envelope_json is null)),
  constraint connector_object_upload_jobs_completion_valid
    check((status='succeeded')=(completed_at is not null)
      and (status<>'succeeded' or remote_resource_id is not null)),
  constraint connector_object_upload_jobs_project_idempotency_unique
    unique(project_id,connector_instance_id,operation_kind,idempotency_key),
  constraint connector_object_upload_jobs_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create index connector_object_upload_jobs_pending_idx
  on connector_object_upload_jobs(connector_instance_id,operation_kind,status,updated_at)
  where status not in('succeeded','failed');
