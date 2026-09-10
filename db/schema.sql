--
-- PostgreSQL database dump
--

-- Dumped from database version 17.5 (Debian 17.5-1.pgdg110+1)
-- Dumped by pg_dump version 17.5 (Debian 17.5-1.pgdg110+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET search_path = public;
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: postgis; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS postgis WITH SCHEMA public;


--
-- Name: EXTENSION postgis; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION postgis IS 'PostGIS geometry and geography spatial types and functions';


--
-- Name: notify_aerosight_outbox(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.notify_aerosight_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  perform pg_notify('aerosight_outbox', json_build_object(
    'projectId', new.project_id,
    'eventId', new.event_id
  )::text);
  return new;
end;
$$;


--
-- Name: notify_aerosight_project_event(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.notify_aerosight_project_event() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  perform pg_notify('aerosight_project_events', json_build_object(
    'projectId', new.project_id,
    'cursor', new.cursor
  )::text);
  return new;
end;
$$;


--
-- Name: populate_device_adapter_connector_definition(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.populate_device_adapter_connector_definition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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


--
-- Name: populate_device_capability_type_driver(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.populate_device_capability_type_driver() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  if new.device_type_id is null or new.driver_definition_id is null then
    select device.device_type_id, device_type.driver_definition_id
      into new.device_type_id, new.driver_definition_id
      from devices device
      join device_types device_type on device_type.id = device.device_type_id
     where device.id = new.device_id and device.project_id = new.project_id;
  end if;
  return new;
end;
$$;


--
-- Name: populate_device_stream_channel_stable_id(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.populate_device_stream_channel_stable_id() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  if new.stable_channel_id is null or length(trim(new.stable_channel_id)) = 0 then
    new.stable_channel_id := 'device:' || new.project_id || ':' || new.device_id || ':' || new.channel_key;
  end if;
  return new;
end;
$$;


--
-- Name: project_approval_request_status(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.project_approval_request_status() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
declare required_count integer;
declare approved_count integer;
begin
  if new.decision = 'rejected' then
    update approval_requests set status = 'rejected', decided_at = new.decided_at
      where id = new.approval_request_id;
    return new;
  end if;
  select required_approvals into required_count from approval_requests where id = new.approval_request_id;
  select count(*) into approved_count from approvals
    where approval_request_id = new.approval_request_id and decision = 'approved';
  if approved_count >= required_count then
    update approval_requests set status = 'approved', decided_at = new.decided_at
      where id = new.approval_request_id;
  end if;
  return new;
end;
$$;


--
-- Name: protect_published_algorithm_definition_version(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.protect_published_algorithm_definition_version() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  if tg_op = 'DELETE' and old.status in ('published','retired') then
    raise exception 'published algorithm definition versions are immutable' using errcode = '55000';
  end if;
  if tg_op = 'UPDATE' and old.status = 'published' then
    if new.status = 'retired'
       and new.execution_mode = old.execution_mode
       and new.model_or_process = old.model_or_process
       and new.input_requirements_json = old.input_requirements_json
       and new.parameters_schema_json = old.parameters_schema_json
       and new.protocol_config_json = old.protocol_config_json
       and new.output_mapping_json = old.output_mapping_json
       and new.label_mapping_json = old.label_mapping_json
       and new.output_schema_json = old.output_schema_json
       and new.display_metadata_json = old.display_metadata_json
       and new.publish_threshold = old.publish_threshold then
      return new;
    end if;
    raise exception 'published algorithm definition versions are immutable' using errcode = '55000';
  end if;
  if tg_op = 'UPDATE' and old.status = 'retired' then
    raise exception 'published algorithm definition versions are immutable' using errcode = '55000';
  end if;
  return case when tg_op = 'DELETE' then old else new end;
end;
$$;


--
-- Name: protect_published_event_rule_version(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.protect_published_event_rule_version() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ begin
  if old.status in ('published','retired') then raise exception 'published event rule versions are immutable' using errcode='55000'; end if;
  return case when tg_op='DELETE' then old else new end;
end; $$;


--
-- Name: protect_published_evidence_link(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.protect_published_evidence_link() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  if old.is_published then
    raise exception 'published evidence links are immutable' using errcode = '55000';
  end if;
  return case when tg_op = 'DELETE' then old else new end;
end;
$$;


--
-- Name: protect_published_retention_policy(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.protect_published_retention_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  if old.status='published' then
    raise exception 'published retention policy is immutable' using errcode='55000';
  end if;
  return case when tg_op='DELETE' then old else new end;
end;
$$;


--
-- Name: protect_published_safety_policy_version(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.protect_published_safety_policy_version() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  if old.status = 'published' then
    raise exception 'published safety policy versions are immutable' using errcode = '55000';
  end if;
  return case when tg_op = 'DELETE' then old else new end;
end;
$$;


--
-- Name: protect_published_task_step(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.protect_published_task_step() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  if exists (select 1 from task_versions where id = old.task_version_id and status in ('published', 'retired')) then
    raise exception 'published task steps are immutable' using errcode = '55000';
  end if;
  return case when tg_op = 'DELETE' then old else new end;
end;
$$;


--
-- Name: protect_published_task_version(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.protect_published_task_version() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  if old.status in ('published', 'retired') then
    raise exception 'published task versions are immutable' using errcode = '55000';
  end if;
  return case when tg_op = 'DELETE' then old else new end;
end;
$$;


--
-- Name: provision_project_copilot_agent(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.provision_project_copilot_agent() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
  insert into agents(project_id,name,description,status,config_json)
  values(new.id,'Copilot','项目级 AI 助手，可通过案件评论提及或负责人指派触发。','active',
         '{"kind":"copilot","builtIn":true}'::jsonb);
  return new;
end $$;


--
-- Name: validate_approval_decision(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.validate_approval_decision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
declare request approval_requests%rowtype;
begin
  select * into request from approval_requests where id = new.approval_request_id for update;
  if request.status <> 'pending' then
    raise exception 'approval request is not pending' using errcode = '55000';
  end if;
  if request.expires_at <= new.decided_at then
    raise exception 'approval request expired' using errcode = '55000';
  end if;
  if request.require_separation and request.requested_by_user_id = new.approver_user_id then
    raise exception 'requester cannot approve own request' using errcode = '42501';
  end if;
  return new;
end;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: agent_draft_evidence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_draft_evidence (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    agent_draft_id uuid NOT NULL,
    reference_type text NOT NULL,
    reference_id text NOT NULL,
    reference_version text NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    quality text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT agent_draft_evidence_type_valid CHECK ((reference_type = ANY (ARRAY['asset'::text, 'event'::text, 'detection'::text, 'track'::text, 'task_run'::text])))
);


--
-- Name: agent_draft_evidence_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.agent_draft_evidence_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: agent_draft_evidence_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.agent_draft_evidence_id_seq OWNED BY public.agent_draft_evidence.id;


--
-- Name: agent_drafts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_drafts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    session_id integer NOT NULL,
    created_by_user_id integer NOT NULL,
    draft_type text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    title text NOT NULL,
    payload_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    model_id text,
    prompt_template_version text,
    generation_tool_calls_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    evidence_version_hash text,
    generated_at timestamp with time zone,
    CONSTRAINT agent_drafts_generation_metadata_complete CHECK ((((model_id IS NULL) AND (prompt_template_version IS NULL) AND (evidence_version_hash IS NULL) AND (generated_at IS NULL)) OR ((model_id IS NOT NULL) AND (prompt_template_version IS NOT NULL) AND (evidence_version_hash IS NOT NULL) AND (generated_at IS NOT NULL)))),
    CONSTRAINT agent_drafts_status_valid CHECK ((status = ANY (ARRAY['draft'::text, 'discarded'::text, 'published'::text]))),
    CONSTRAINT agent_drafts_type_valid CHECK ((draft_type = ANY (ARRAY['inspection_task'::text, 'report'::text, 'issue'::text])))
);


--
-- Name: agent_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_messages (
    id integer NOT NULL,
    session_id integer NOT NULL,
    role text NOT NULL,
    content text NOT NULL,
    tool_calls_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    token_usage_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: agent_messages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.agent_messages_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: agent_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.agent_messages_id_seq OWNED BY public.agent_messages.id;


--
-- Name: agent_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_sessions (
    id integer NOT NULL,
    project_id integer NOT NULL,
    agent_id integer,
    task_run_id integer,
    issue_id integer,
    status text DEFAULT 'open'::text NOT NULL,
    started_by_user_id integer,
    summary text,
    started_at timestamp without time zone DEFAULT now() NOT NULL,
    ended_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: agent_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.agent_sessions_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: agent_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.agent_sessions_id_seq OWNED BY public.agent_sessions.id;


--
-- Name: agent_tool_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_tool_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    session_id integer NOT NULL,
    requested_by_user_id integer NOT NULL,
    tool_name text NOT NULL,
    required_permission text NOT NULL,
    args_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    context_expires_at timestamp with time zone NOT NULL,
    authorization_checked_at timestamp with time zone,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    failure_code text,
    result_json jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    issue_id integer,
    trigger_issue_event_id integer,
    trigger_type text,
    idempotency_key text,
    CONSTRAINT agent_tool_jobs_expiry_valid CHECK ((context_expires_at > created_at)),
    CONSTRAINT agent_tool_jobs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text]))),
    CONSTRAINT agent_tool_jobs_trigger_type_valid CHECK (((trigger_type IS NULL) OR (trigger_type = ANY (ARRAY['issue_mention'::text, 'issue_assignment'::text, 'task_step'::text, 'chat'::text]))))
);


--
-- Name: agents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agents (
    id integer NOT NULL,
    project_id integer NOT NULL,
    name text NOT NULL,
    description text,
    status text DEFAULT 'disabled'::text NOT NULL,
    config_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: agents_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.agents_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: agents_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.agents_id_seq OWNED BY public.agents.id;


--
-- Name: ai_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_providers (
    id bigint NOT NULL,
    name text NOT NULL,
    provider_type text NOT NULL,
    base_url text,
    model_id text NOT NULL,
    credential_envelope_json jsonb NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    status text DEFAULT 'untested'::text NOT NULL,
    health_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    last_tested_at timestamp with time zone,
    created_by_user_id integer NOT NULL,
    updated_by_user_id integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_providers_credential_envelope_object CHECK ((jsonb_typeof(credential_envelope_json) = 'object'::text)),
    CONSTRAINT ai_providers_default_enabled CHECK (((NOT is_default) OR enabled)),
    CONSTRAINT ai_providers_status_valid CHECK ((status = ANY (ARRAY['untested'::text, 'healthy'::text, 'degraded'::text, 'failed'::text]))),
    CONSTRAINT ai_providers_type_valid CHECK ((provider_type = 'openai'::text))
);


--
-- Name: ai_providers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ai_providers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_providers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ai_providers_id_seq OWNED BY public.ai_providers.id;


--
-- Name: alert_automation_drafts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.alert_automation_drafts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    automation_run_id uuid NOT NULL,
    perception_event_id uuid NOT NULL,
    draft_type text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    title text NOT NULL,
    payload_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    evidence_refs_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT alert_automation_drafts_status_valid CHECK ((status = ANY (ARRAY['draft'::text, 'discarded'::text, 'published'::text]))),
    CONSTRAINT alert_automation_drafts_type_valid CHECK ((draft_type = ANY (ARRAY['report'::text, 'issue'::text, 'follow-up-task'::text])))
);


--
-- Name: alert_automation_policies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.alert_automation_policies (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    name text NOT NULL,
    current_published_version_id bigint,
    created_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: alert_automation_policies_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.alert_automation_policies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: alert_automation_policies_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.alert_automation_policies_id_seq OWNED BY public.alert_automation_policies.id;


--
-- Name: alert_automation_policy_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.alert_automation_policy_versions (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    alert_automation_policy_id bigint NOT NULL,
    event_rule_version_id bigint,
    version integer NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    mode text DEFAULT 'manual'::text NOT NULL,
    config_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by_user_id integer,
    published_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    CONSTRAINT alert_automation_policy_versions_mode_valid CHECK ((mode = ANY (ARRAY['manual'::text, 'agent-on-demand'::text, 'agent-auto-draft'::text, 'follow-up-draft'::text]))),
    CONSTRAINT alert_automation_policy_versions_status_valid CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text, 'retired'::text]))),
    CONSTRAINT alert_automation_policy_versions_version_positive CHECK ((version > 0))
);


--
-- Name: alert_automation_policy_versions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.alert_automation_policy_versions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: alert_automation_policy_versions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.alert_automation_policy_versions_id_seq OWNED BY public.alert_automation_policy_versions.id;


--
-- Name: alert_automation_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.alert_automation_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    policy_version_id bigint NOT NULL,
    perception_event_id uuid NOT NULL,
    trigger_reason text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    input_scope_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    output_refs_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    failure_code text,
    failure_message text,
    queued_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT alert_automation_runs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'canceled'::text])))
);


--
-- Name: algorithm_callback_receipts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.algorithm_callback_receipts (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    algorithm_run_id uuid NOT NULL,
    provider_id bigint NOT NULL,
    callback_id text NOT NULL,
    external_job_id text NOT NULL,
    payload_hash text NOT NULL,
    disposition text DEFAULT 'verified'::text NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT algorithm_callback_receipts_disposition_valid CHECK ((disposition = ANY (ARRAY['verified'::text, 'applied'::text]))),
    CONSTRAINT algorithm_callback_receipts_hash_valid CHECK ((payload_hash ~ '^[a-f0-9]{64}$'::text))
);


--
-- Name: algorithm_callback_receipts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.algorithm_callback_receipts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: algorithm_callback_receipts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.algorithm_callback_receipts_id_seq OWNED BY public.algorithm_callback_receipts.id;


--
-- Name: algorithm_definition_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.algorithm_definition_versions (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    algorithm_definition_id bigint NOT NULL,
    version integer NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    execution_mode text NOT NULL,
    model_or_process text NOT NULL,
    input_requirements_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    parameters_schema_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    protocol_config_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    output_mapping_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    label_mapping_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    publish_threshold double precision DEFAULT 0 NOT NULL,
    created_by_user_id integer,
    published_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    output_schema_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    display_metadata_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT algorithm_definition_versions_mode_valid CHECK ((execution_mode = ANY (ARRAY['synchronous'::text, 'asynchronous'::text, 'callback'::text]))),
    CONSTRAINT algorithm_definition_versions_status_valid CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text, 'retired'::text]))),
    CONSTRAINT algorithm_definition_versions_threshold_valid CHECK (((publish_threshold >= (0)::double precision) AND (publish_threshold <= (1)::double precision))),
    CONSTRAINT algorithm_definition_versions_version_valid CHECK ((version > 0))
);


--
-- Name: algorithm_definition_versions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.algorithm_definition_versions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: algorithm_definition_versions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.algorithm_definition_versions_id_seq OWNED BY public.algorithm_definition_versions.id;


--
-- Name: algorithm_definitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.algorithm_definitions (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    provider_id bigint NOT NULL,
    name text NOT NULL,
    capability_code text NOT NULL,
    description text,
    current_published_version_id bigint,
    created_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: algorithm_definitions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.algorithm_definitions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: algorithm_definitions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.algorithm_definitions_id_seq OWNED BY public.algorithm_definitions.id;


--
-- Name: algorithm_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.algorithm_providers (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    name text NOT NULL,
    provider_type text NOT NULL,
    base_url text NOT NULL,
    secret_ref text,
    auth_type text DEFAULT 'none'::text NOT NULL,
    allowed_headers_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    timeout_seconds integer DEFAULT 30 NOT NULL,
    concurrency_limit integer DEFAULT 1 NOT NULL,
    rate_limit_per_minute integer DEFAULT 60 NOT NULL,
    status text DEFAULT 'disabled'::text NOT NULL,
    health_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    credential_envelope_json jsonb,
    CONSTRAINT algorithm_providers_auth_valid CHECK ((auth_type = ANY (ARRAY['none'::text, 'bearer'::text, 'api-key-header'::text, 'basic'::text, 'signed'::text]))),
    CONSTRAINT algorithm_providers_credential_envelope_object CHECK (((credential_envelope_json IS NULL) OR (jsonb_typeof(credential_envelope_json) = 'object'::text))),
    CONSTRAINT algorithm_providers_limits_valid CHECK ((((timeout_seconds >= 1) AND (timeout_seconds <= 3600)) AND (concurrency_limit > 0) AND (rate_limit_per_minute > 0))),
    CONSTRAINT algorithm_providers_status_valid CHECK ((status = ANY (ARRAY['disabled'::text, 'testing'::text, 'active'::text, 'degraded'::text, 'failed'::text]))),
    CONSTRAINT algorithm_providers_type_valid CHECK ((provider_type = ANY (ARRAY['http-json'::text, 'kserve-v2'::text, 'ogc-processes'::text, 'ai-sdk'::text])))
);


--
-- Name: algorithm_providers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.algorithm_providers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: algorithm_providers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.algorithm_providers_id_seq OWNED BY public.algorithm_providers.id;


--
-- Name: algorithm_run_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.algorithm_run_attempts (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    algorithm_run_id uuid NOT NULL,
    attempt integer NOT NULL,
    status text NOT NULL,
    request_hash text NOT NULL,
    response_status integer,
    external_job_id text,
    duration_ms integer,
    error_category text,
    billing_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    CONSTRAINT algorithm_run_attempts_attempt_valid CHECK (((attempt > 0) AND ((duration_ms IS NULL) OR (duration_ms >= 0)))),
    CONSTRAINT algorithm_run_attempts_status_valid CHECK ((status = ANY (ARRAY['running'::text, 'succeeded'::text, 'failed'::text, 'timed_out'::text, 'rate_limited'::text])))
);


--
-- Name: algorithm_run_attempts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.algorithm_run_attempts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: algorithm_run_attempts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.algorithm_run_attempts_id_seq OWNED BY public.algorithm_run_attempts.id;


--
-- Name: algorithm_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.algorithm_runs (
    id uuid NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    algorithm_definition_version_id bigint NOT NULL,
    input_asset_id integer NOT NULL,
    task_run_id integer,
    device_id integer,
    idempotency_key text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    parameters_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    input_snapshot_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    external_job_id text,
    callback_token_hash text,
    raw_result_object_key text,
    raw_result_checksum_sha256 text,
    canonical_result_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    error_code text,
    error_message text,
    task_run_step_id bigint,
    CONSTRAINT algorithm_runs_checksum_valid CHECK (((raw_result_checksum_sha256 IS NULL) OR (raw_result_checksum_sha256 ~ '^[a-f0-9]{64}$'::text))),
    CONSTRAINT algorithm_runs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'polling'::text, 'waiting_callback'::text, 'succeeded'::text, 'failed'::text, 'canceled'::text, 'timed_out'::text])))
);


--
-- Name: approval_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_requests (
    id uuid NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    action text NOT NULL,
    requested_by_user_id integer NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    required_approvals integer DEFAULT 1 NOT NULL,
    require_separation boolean DEFAULT true NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    context_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    decided_at timestamp with time zone,
    CONSTRAINT approval_requests_required_valid CHECK ((required_approvals > 0)),
    CONSTRAINT approval_requests_status_valid CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'expired'::text])))
);


--
-- Name: approvals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approvals (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    approval_request_id uuid NOT NULL,
    approver_user_id integer NOT NULL,
    decision text NOT NULL,
    reason text NOT NULL,
    decided_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT approvals_decision_valid CHECK ((decision = ANY (ARRAY['approved'::text, 'rejected'::text])))
);


--
-- Name: approvals_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.approvals_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: approvals_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.approvals_id_seq OWNED BY public.approvals.id;


--
-- Name: asset_derivatives; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.asset_derivatives (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    source_asset_id integer NOT NULL,
    derived_asset_id integer NOT NULL,
    derivative_type text NOT NULL,
    generator text NOT NULL,
    generator_version text,
    parameters_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT asset_derivatives_not_self CHECK ((source_asset_id <> derived_asset_id))
);


--
-- Name: asset_derivatives_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.asset_derivatives_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: asset_derivatives_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.asset_derivatives_id_seq OWNED BY public.asset_derivatives.id;


--
-- Name: asset_upload_intents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.asset_upload_intents (
    id uuid NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    actor_user_id integer,
    logical_key text NOT NULL,
    object_key text NOT NULL,
    file_name text NOT NULL,
    kind text NOT NULL,
    mime_type text NOT NULL,
    expected_size_bytes bigint NOT NULL,
    expected_checksum_sha256 text NOT NULL,
    device_id integer,
    task_run_id integer,
    issue_id integer,
    status text DEFAULT 'pending'::text NOT NULL,
    asset_id integer,
    failure_code text,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT asset_upload_intents_checksum_valid CHECK ((expected_checksum_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT asset_upload_intents_size_valid CHECK ((expected_size_bytes >= 0)),
    CONSTRAINT asset_upload_intents_status_valid CHECK ((status = ANY (ARRAY['pending'::text, 'completed'::text, 'failed'::text, 'expired'::text])))
);


--
-- Name: assets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.assets (
    id integer NOT NULL,
    project_id integer NOT NULL,
    device_id integer,
    task_run_id integer,
    issue_id integer,
    kind text NOT NULL,
    mime_type text,
    storage_key text NOT NULL,
    size_bytes bigint,
    checksum text,
    captured_at timestamp without time zone,
    metadata_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    team_id integer NOT NULL,
    logical_key text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    status text DEFAULT 'available'::text NOT NULL,
    object_version text,
    checksum_sha256 text,
    available_at timestamp with time zone,
    failed_at timestamp with time zone,
    failure_code text,
    retention_hold_until timestamp with time zone,
    legal_hold boolean DEFAULT false NOT NULL,
    retention_reason text,
    deleted_at timestamp with time zone,
    supersedes_asset_id integer,
    CONSTRAINT assets_checksum_sha256_valid CHECK (((checksum_sha256 IS NULL) OR (checksum_sha256 ~ '^[a-f0-9]{64}$'::text))),
    CONSTRAINT assets_status_valid CHECK ((status = ANY (ARRAY['pending'::text, 'available'::text, 'failed'::text, 'deleted'::text]))),
    CONSTRAINT assets_version_positive CHECK ((version > 0))
);


--
-- Name: assets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.assets_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: assets_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.assets_id_seq OWNED BY public.assets.id;


--
-- Name: audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_events (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    request_id text NOT NULL,
    idempotency_key text,
    actor_user_id integer,
    actor_agent_id integer,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    input_hash text NOT NULL,
    policy_result_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    result_hash text,
    status text DEFAULT 'accepted'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT audit_events_actor_present CHECK (((actor_user_id IS NOT NULL) OR (actor_agent_id IS NOT NULL))),
    CONSTRAINT audit_events_status_valid CHECK ((status = ANY (ARRAY['accepted'::text, 'completed'::text])))
);


--
-- Name: audit_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audit_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audit_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audit_events_id_seq OWNED BY public.audit_events.id;


--
-- Name: command_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.command_attempts (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    command_id uuid NOT NULL,
    adapter_id bigint NOT NULL,
    attempt integer NOT NULL,
    status text NOT NULL,
    sent_at timestamp with time zone DEFAULT now() NOT NULL,
    acknowledged_at timestamp with time zone,
    error_code text,
    result_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT command_attempts_attempt_valid CHECK ((attempt > 0)),
    CONSTRAINT command_attempts_status_valid CHECK ((status = ANY (ARRAY['sent'::text, 'acknowledged'::text, 'nacked'::text, 'timed_out'::text, 'transport_error'::text])))
);


--
-- Name: command_attempts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.command_attempts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: command_attempts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.command_attempts_id_seq OWNED BY public.command_attempts.id;


--
-- Name: connector_action_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_action_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    task_run_id integer NOT NULL,
    device_id integer NOT NULL,
    wayline_resource_id bigint,
    target_resource_id bigint,
    remote_result_resource_id bigint,
    approval_request_id uuid NOT NULL,
    requested_by_user_id integer NOT NULL,
    action_kind text NOT NULL,
    idempotency_key text NOT NULL,
    request_digest text NOT NULL,
    request_envelope_json jsonb NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    dispatch_check_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    reconciliation_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    accepted_at timestamp with time zone,
    reconciled_at timestamp with time zone,
    unknown_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_action_jobs_action_valid CHECK ((action_kind = ANY (ARRAY['flight-task-create'::text, 'flight-task-status'::text, 'flight-task-resumption'::text]))),
    CONSTRAINT connector_action_jobs_attempts_valid CHECK ((((attempt_count >= 0) AND (attempt_count <= 1)) AND ((reconciliation_count >= 0) AND (reconciliation_count <= 8)))),
    CONSTRAINT connector_action_jobs_completion_valid CHECK ((((status = 'succeeded'::text) = (completed_at IS NOT NULL)) AND ((status <> 'succeeded'::text) OR (remote_result_resource_id IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL)))),
    CONSTRAINT connector_action_jobs_digest_valid CHECK ((request_digest ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT connector_action_jobs_dispatch_object CHECK ((jsonb_typeof(dispatch_check_json) = 'object'::text)),
    CONSTRAINT connector_action_jobs_envelope_object CHECK ((jsonb_typeof(request_envelope_json) = 'object'::text)),
    CONSTRAINT connector_action_jobs_idempotency_valid CHECK ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key)))),
    CONSTRAINT connector_action_jobs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'prepared'::text, 'reconciling'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text]))),
    CONSTRAINT connector_action_jobs_target_shape CHECK ((((action_kind = 'flight-task-create'::text) AND (wayline_resource_id IS NOT NULL) AND (target_resource_id IS NULL)) OR ((action_kind = ANY (ARRAY['flight-task-status'::text, 'flight-task-resumption'::text])) AND (wayline_resource_id IS NULL) AND (target_resource_id IS NOT NULL))))
);


--
-- Name: connector_asset_access_refs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_asset_access_refs (
    id integer NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    remote_resource_id bigint NOT NULL,
    access_kind text NOT NULL,
    reference_digest text NOT NULL,
    credential_envelope_json jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_asset_access_refs_digest_valid CHECK ((reference_digest ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT connector_asset_access_refs_envelope_object CHECK ((jsonb_typeof(credential_envelope_json) = 'object'::text)),
    CONSTRAINT connector_asset_access_refs_kind_valid CHECK ((access_kind = ANY (ARRAY['flight-media'::text, 'flight-record'::text, 'model'::text, 'model-resource'::text])))
);


--
-- Name: connector_capability_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_capability_snapshots (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    capability_code text NOT NULL,
    status text NOT NULL,
    evidence_level text NOT NULL,
    region text NOT NULL,
    deployment text NOT NULL,
    device_model text,
    firmware_version text,
    details_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    verified_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    account_fingerprint text,
    CONSTRAINT connector_capability_snapshots_account_fingerprint_valid CHECK (((account_fingerprint IS NULL) OR (account_fingerprint ~ '^[a-f0-9]{64}$'::text))),
    CONSTRAINT connector_capability_snapshots_details_object CHECK ((jsonb_typeof(details_json) = 'object'::text)),
    CONSTRAINT connector_capability_snapshots_evidence_valid CHECK ((evidence_level = ANY (ARRAY['documented'::text, 'fixture'::text, 'live-read'::text, 'field-write'::text]))),
    CONSTRAINT connector_capability_snapshots_identity_valid CHECK ((((length(btrim(capability_code)) >= 1) AND (length(btrim(capability_code)) <= 256)) AND (capability_code = btrim(capability_code)) AND ((length(btrim(region)) >= 1) AND (length(btrim(region)) <= 64)) AND ((length(btrim(deployment)) >= 1) AND (length(btrim(deployment)) <= 128)))),
    CONSTRAINT connector_capability_snapshots_status_valid CHECK ((status = ANY (ARRAY['supported'::text, 'empty'::text, 'forbidden'::text, 'not_applicable'::text, 'unverified'::text, 'degraded'::text, 'failed'::text]))),
    CONSTRAINT connector_capability_snapshots_time_valid CHECK (((expires_at IS NULL) OR (expires_at > verified_at)))
);


--
-- Name: connector_capability_snapshots_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.connector_capability_snapshots_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: connector_capability_snapshots_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.connector_capability_snapshots_id_seq OWNED BY public.connector_capability_snapshots.id;


--
-- Name: connector_control_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_control_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    device_id integer NOT NULL,
    holder_user_id integer NOT NULL,
    approval_request_id uuid NOT NULL,
    safety_policy_version_id bigint NOT NULL,
    idempotency_key text NOT NULL,
    controls_json jsonb NOT NULL,
    status text DEFAULT 'requested'::text NOT NULL,
    acquire_attempt_count integer DEFAULT 0 NOT NULL,
    release_attempt_count integer DEFAULT 0 NOT NULL,
    last_heartbeat_at timestamp with time zone NOT NULL,
    lease_expires_at timestamp with time zone NOT NULL,
    absolute_expires_at timestamp with time zone NOT NULL,
    last_operation_at timestamp with time zone,
    operation_window_started_at timestamp with time zone NOT NULL,
    operation_count integer DEFAULT 0 NOT NULL,
    failure_code text,
    acquired_at timestamp with time zone,
    release_requested_at timestamp with time zone,
    released_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_control_sessions_attempts_valid CHECK ((((acquire_attempt_count >= 0) AND (acquire_attempt_count <= 1)) AND ((release_attempt_count >= 0) AND (release_attempt_count <= 1)))),
    CONSTRAINT connector_control_sessions_controls_object CHECK ((jsonb_typeof(controls_json) = 'object'::text)),
    CONSTRAINT connector_control_sessions_idempotency_valid CHECK ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key)))),
    CONSTRAINT connector_control_sessions_lease_valid CHECK (((lease_expires_at <= absolute_expires_at) AND (absolute_expires_at <= (created_at + '00:10:00'::interval)))),
    CONSTRAINT connector_control_sessions_operation_count_valid CHECK (((operation_count >= 0) AND (operation_count <= 30))),
    CONSTRAINT connector_control_sessions_status_valid CHECK ((status = ANY (ARRAY['requested'::text, 'acquiring'::text, 'active'::text, 'releasing'::text, 'released'::text, 'failed'::text, 'expired'::text]))),
    CONSTRAINT connector_control_sessions_terminal_valid CHECK (((status = ANY (ARRAY['released'::text, 'failed'::text, 'expired'::text])) = (released_at IS NOT NULL)))
);


--
-- Name: connector_definitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_definitions (
    id bigint NOT NULL,
    connector_key text NOT NULL,
    version text NOT NULL,
    display_name text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    manifest_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_definitions_manifest_object CHECK ((jsonb_typeof(manifest_json) = 'object'::text)),
    CONSTRAINT connector_definitions_status_valid CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text, 'retired'::text])))
);


--
-- Name: connector_definitions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.connector_definitions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: connector_definitions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.connector_definitions_id_seq OWNED BY public.connector_definitions.id;


--
-- Name: connector_device_admin_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_device_admin_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    device_id integer,
    requested_by_user_id integer NOT NULL,
    approval_request_id uuid NOT NULL,
    action_kind text NOT NULL,
    capability_code text NOT NULL,
    feature_flag text NOT NULL,
    idempotency_key text NOT NULL,
    request_digest text NOT NULL,
    request_envelope_json jsonb NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    result_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    result_envelope_json jsonb,
    attempted_at timestamp with time zone,
    unknown_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_device_admin_jobs_action_valid CHECK ((action_kind = ANY (ARRAY['rtk-calibrate'::text, 'relay-pair'::text, 'active-project-update'::text, 'sn-decrypt'::text]))),
    CONSTRAINT connector_device_admin_jobs_attempt_valid CHECK (((attempt_count >= 0) AND (attempt_count <= 1))),
    CONSTRAINT connector_device_admin_jobs_completion_valid CHECK ((((status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text])) = (completed_at IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL)))),
    CONSTRAINT connector_device_admin_jobs_digest_valid CHECK ((request_digest ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT connector_device_admin_jobs_envelopes_valid CHECK (((jsonb_typeof(request_envelope_json) = 'object'::text) AND ((result_envelope_json IS NULL) OR (jsonb_typeof(result_envelope_json) = 'object'::text)))),
    CONSTRAINT connector_device_admin_jobs_idempotency_valid CHECK ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key)))),
    CONSTRAINT connector_device_admin_jobs_policy_valid CHECK ((((action_kind = 'rtk-calibrate'::text) AND (capability_code = 'device.rtk.calibrate'::text) AND (feature_flag = 'flighthub.rtk.calibrate'::text)) OR ((action_kind = 'relay-pair'::text) AND (capability_code = 'device.relay.pair'::text) AND (feature_flag = 'flighthub.relay.pair'::text)) OR ((action_kind = 'active-project-update'::text) AND (capability_code = 'device.active-project.update'::text) AND (feature_flag = 'flighthub.device-migration'::text)) OR ((action_kind = 'sn-decrypt'::text) AND (capability_code = 'security.sn.decrypt'::text) AND (feature_flag = 'flighthub.sn-decrypt'::text)))),
    CONSTRAINT connector_device_admin_jobs_result_valid CHECK ((jsonb_typeof(result_json) = 'object'::text)),
    CONSTRAINT connector_device_admin_jobs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'executing'::text, 'accepted'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text]))),
    CONSTRAINT connector_device_admin_jobs_target_valid CHECK (((action_kind = 'sn-decrypt'::text) = (device_id IS NULL)))
);


--
-- Name: connector_geospatial_action_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_geospatial_action_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    target_resource_id bigint,
    requested_by_user_id integer NOT NULL,
    action_kind text NOT NULL,
    capability_code text NOT NULL,
    feature_flag text NOT NULL,
    idempotency_key text NOT NULL,
    expected_remote_version text,
    request_digest text NOT NULL,
    request_envelope_json jsonb NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    result_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    attempted_at timestamp with time zone,
    unknown_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_geospatial_action_jobs_action_valid CHECK ((action_kind = ANY (ARRAY['map-element-create'::text, 'map-element-update'::text, 'map-element-delete'::text]))),
    CONSTRAINT connector_geospatial_action_jobs_attempt_valid CHECK (((attempt_count >= 0) AND (attempt_count <= 1))),
    CONSTRAINT connector_geospatial_action_jobs_capability_valid CHECK ((((action_kind = ANY (ARRAY['map-element-create'::text, 'map-element-update'::text])) AND (capability_code = 'geospatial.write'::text) AND (feature_flag = 'flighthub.actions'::text)) OR ((action_kind = 'map-element-delete'::text) AND (capability_code = 'geospatial.element.delete'::text) AND (feature_flag = 'flighthub.geospatial.delete'::text)))),
    CONSTRAINT connector_geospatial_action_jobs_completion_valid CHECK ((((status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text])) = (completed_at IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL)))),
    CONSTRAINT connector_geospatial_action_jobs_digest_valid CHECK ((request_digest ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT connector_geospatial_action_jobs_envelope_object CHECK ((jsonb_typeof(request_envelope_json) = 'object'::text)),
    CONSTRAINT connector_geospatial_action_jobs_idempotency_valid CHECK ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key)))),
    CONSTRAINT connector_geospatial_action_jobs_result_object CHECK ((jsonb_typeof(result_json) = 'object'::text)),
    CONSTRAINT connector_geospatial_action_jobs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'executing'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text]))),
    CONSTRAINT connector_geospatial_action_jobs_target_shape CHECK ((((action_kind = 'map-element-create'::text) AND (target_resource_id IS NULL) AND (expected_remote_version IS NULL)) OR ((action_kind = ANY (ARRAY['map-element-update'::text, 'map-element-delete'::text])) AND (target_resource_id IS NOT NULL) AND ((length(btrim(expected_remote_version)) >= 1) AND (length(btrim(expected_remote_version)) <= 512)) AND (expected_remote_version = btrim(expected_remote_version)))))
);


--
-- Name: device_adapters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_adapters (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    name text NOT NULL,
    adapter_type text NOT NULL,
    vendor text,
    protocol_version text DEFAULT '1'::text NOT NULL,
    status text DEFAULT 'disabled'::text NOT NULL,
    secret_ref text,
    config_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    capabilities_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    last_health_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    last_checked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    network_profile_id bigint,
    lease_owner text,
    lease_expires_at timestamp with time zone,
    connection_epoch bigint DEFAULT 0 NOT NULL,
    last_connected_at timestamp with time zone,
    connector_definition_id bigint NOT NULL,
    onboarding_policy text DEFAULT 'review'::text NOT NULL,
    discovery_scope_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    sync_cursor_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    credential_envelope_json jsonb,
    external_scope_key text,
    CONSTRAINT device_adapters_credential_envelope_object CHECK (((credential_envelope_json IS NULL) OR (jsonb_typeof(credential_envelope_json) = 'object'::text))),
    CONSTRAINT device_adapters_discovery_scope_object CHECK ((jsonb_typeof(discovery_scope_json) = 'object'::text)),
    CONSTRAINT device_adapters_external_scope_key_normalized CHECK (((external_scope_key IS NULL) OR (((length(external_scope_key) >= 1) AND (length(external_scope_key) <= 512)) AND (external_scope_key = btrim(external_scope_key))))),
    CONSTRAINT device_adapters_lease_complete CHECK ((((lease_owner IS NULL) AND (lease_expires_at IS NULL)) OR ((lease_owner IS NOT NULL) AND (lease_expires_at IS NOT NULL)))),
    CONSTRAINT device_adapters_onboarding_policy_valid CHECK ((onboarding_policy = ANY (ARRAY['automatic'::text, 'review'::text, 'observe-only'::text]))),
    CONSTRAINT device_adapters_status_valid CHECK ((status = ANY (ARRAY['disabled'::text, 'connecting'::text, 'connected'::text, 'degraded'::text, 'failed'::text]))),
    CONSTRAINT device_adapters_sync_cursor_object CHECK ((jsonb_typeof(sync_cursor_json) = 'object'::text))
);


--
-- Name: connector_instances; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.connector_instances AS
 SELECT adapter.id,
    adapter.project_id,
    adapter.team_id,
    adapter.name,
    adapter.connector_definition_id,
    definition.connector_key,
    definition.version AS connector_version,
    adapter.adapter_type AS legacy_adapter_type,
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
   FROM (public.device_adapters adapter
     JOIN public.connector_definitions definition ON ((definition.id = adapter.connector_definition_id)));


--
-- Name: connector_live_action_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_live_action_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    device_id integer,
    target_resource_id bigint,
    requested_by_user_id integer NOT NULL,
    action_kind text NOT NULL,
    capability_code text NOT NULL,
    feature_flag text NOT NULL,
    idempotency_key text NOT NULL,
    request_digest text NOT NULL,
    request_envelope_json jsonb NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    result_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    attempted_at timestamp with time zone,
    unknown_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_live_action_jobs_action_valid CHECK ((action_kind = ANY (ARRAY['live-quality-set'::text, 'live-converter-create'::text, 'live-converter-toggle'::text, 'live-converter-delete'::text]))),
    CONSTRAINT connector_live_action_jobs_attempt_valid CHECK (((attempt_count >= 0) AND (attempt_count <= 1))),
    CONSTRAINT connector_live_action_jobs_capability_valid CHECK ((((action_kind = 'live-quality-set'::text) AND (capability_code = 'live.quality.set'::text) AND (feature_flag = 'flighthub.live.quality'::text)) OR ((action_kind = 'live-converter-create'::text) AND (capability_code = 'live.converter.create'::text) AND (feature_flag = 'flighthub.live.converter.create'::text)) OR ((action_kind = 'live-converter-toggle'::text) AND (capability_code = 'live.converter.toggle'::text) AND (feature_flag = 'flighthub.live.converter.toggle'::text)) OR ((action_kind = 'live-converter-delete'::text) AND (capability_code = 'live.converter.delete'::text) AND (feature_flag = 'flighthub.live.converter.delete'::text)))),
    CONSTRAINT connector_live_action_jobs_completion_valid CHECK ((((status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text])) = (completed_at IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL)))),
    CONSTRAINT connector_live_action_jobs_digest_valid CHECK ((request_digest ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT connector_live_action_jobs_envelope_object CHECK ((jsonb_typeof(request_envelope_json) = 'object'::text)),
    CONSTRAINT connector_live_action_jobs_idempotency_valid CHECK ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key)))),
    CONSTRAINT connector_live_action_jobs_result_object CHECK ((jsonb_typeof(result_json) = 'object'::text)),
    CONSTRAINT connector_live_action_jobs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'executing'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text]))),
    CONSTRAINT connector_live_action_jobs_target_shape CHECK ((((action_kind = ANY (ARRAY['live-quality-set'::text, 'live-converter-create'::text])) AND (device_id IS NOT NULL) AND (target_resource_id IS NULL)) OR ((action_kind = ANY (ARRAY['live-converter-toggle'::text, 'live-converter-delete'::text])) AND (device_id IS NULL) AND (target_resource_id IS NOT NULL))))
);


--
-- Name: connector_management_write_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_management_write_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    requested_by_user_id integer NOT NULL,
    approval_request_id uuid NOT NULL,
    action_kind text NOT NULL,
    capability_code text NOT NULL,
    feature_flag text NOT NULL,
    idempotency_key text NOT NULL,
    request_digest text NOT NULL,
    request_envelope_json jsonb NOT NULL,
    preview_digest text NOT NULL,
    preview_json jsonb NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    reconciliation_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    result_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    attempted_at timestamp with time zone,
    unknown_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_management_write_jobs_action_valid CHECK ((action_kind = 'project-member-upsert'::text)),
    CONSTRAINT connector_management_write_jobs_attempt_valid CHECK ((((attempt_count >= 0) AND (attempt_count <= 1)) AND ((reconciliation_count >= 0) AND (reconciliation_count <= 1)))),
    CONSTRAINT connector_management_write_jobs_completion_valid CHECK ((((status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text])) = (completed_at IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL)))),
    CONSTRAINT connector_management_write_jobs_digest_valid CHECK (((request_digest ~ '^[a-f0-9]{64}$'::text) AND (preview_digest ~ '^[a-f0-9]{64}$'::text))),
    CONSTRAINT connector_management_write_jobs_idempotency_valid CHECK ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key)))),
    CONSTRAINT connector_management_write_jobs_json_valid CHECK (((jsonb_typeof(request_envelope_json) = 'object'::text) AND (jsonb_typeof(preview_json) = 'object'::text) AND (jsonb_typeof(result_json) = 'object'::text))),
    CONSTRAINT connector_management_write_jobs_policy_valid CHECK (((capability_code = 'organization.project-member.write'::text) AND (feature_flag = 'flighthub.organization.project-member'::text))),
    CONSTRAINT connector_management_write_jobs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'executing'::text, 'accepted'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text])))
);


--
-- Name: connector_model_delete_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_model_delete_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    target_resource_id bigint NOT NULL,
    approval_request_id uuid NOT NULL,
    requested_by_user_id integer NOT NULL,
    action_kind text NOT NULL,
    capability_code text NOT NULL,
    feature_flag text NOT NULL,
    idempotency_key text NOT NULL,
    expected_remote_version text NOT NULL,
    preview_digest text NOT NULL,
    request_digest text NOT NULL,
    request_envelope_json jsonb NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    reconciliation_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    result_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    attempted_at timestamp with time zone,
    unknown_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_model_delete_jobs_action_valid CHECK ((action_kind = ANY (ARRAY['model-delete'::text, 'model-resource-delete'::text]))),
    CONSTRAINT connector_model_delete_jobs_attempts_valid CHECK ((((attempt_count >= 0) AND (attempt_count <= 1)) AND ((reconciliation_count >= 0) AND (reconciliation_count <= 16)))),
    CONSTRAINT connector_model_delete_jobs_completion_valid CHECK ((((status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text])) = (completed_at IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL)))),
    CONSTRAINT connector_model_delete_jobs_digest_valid CHECK (((preview_digest ~ '^[a-f0-9]{64}$'::text) AND (request_digest ~ '^[a-f0-9]{64}$'::text))),
    CONSTRAINT connector_model_delete_jobs_envelope_object CHECK ((jsonb_typeof(request_envelope_json) = 'object'::text)),
    CONSTRAINT connector_model_delete_jobs_idempotency_valid CHECK ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key)))),
    CONSTRAINT connector_model_delete_jobs_policy_valid CHECK ((((action_kind = 'model-delete'::text) AND (capability_code = 'model.delete'::text) AND (feature_flag = 'flighthub.model.delete'::text)) OR ((action_kind = 'model-resource-delete'::text) AND (capability_code = 'model.resource.delete'::text) AND (feature_flag = 'flighthub.model-resource.delete'::text)))),
    CONSTRAINT connector_model_delete_jobs_result_object CHECK ((jsonb_typeof(result_json) = 'object'::text)),
    CONSTRAINT connector_model_delete_jobs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'executing'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text]))),
    CONSTRAINT connector_model_delete_jobs_version_valid CHECK ((((length(btrim(expected_remote_version)) >= 1) AND (length(btrim(expected_remote_version)) <= 512)) AND (expected_remote_version = btrim(expected_remote_version))))
);


--
-- Name: connector_model_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_model_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    requested_by_user_id integer NOT NULL,
    action_kind text NOT NULL,
    idempotency_key text NOT NULL,
    request_digest text NOT NULL,
    request_envelope_json jsonb NOT NULL,
    reconciliation_name text,
    status text DEFAULT 'queued'::text NOT NULL,
    remote_ids_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    asset_ids_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    progress integer DEFAULT 0 NOT NULL,
    stage text DEFAULT 'queued'::text NOT NULL,
    submit_attempt_count integer DEFAULT 0 NOT NULL,
    reconciliation_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    submitted_at timestamp with time zone,
    reconciled_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_model_jobs_action_valid CHECK ((action_kind = ANY (ARRAY['traditional-create'::text, 'open-start'::text, 'open-stop'::text]))),
    CONSTRAINT connector_model_jobs_asset_array CHECK ((jsonb_typeof(asset_ids_json) = 'array'::text)),
    CONSTRAINT connector_model_jobs_attempts_valid CHECK ((((submit_attempt_count >= 0) AND (submit_attempt_count <= 1)) AND ((reconciliation_count >= 0) AND (reconciliation_count <= 32)))),
    CONSTRAINT connector_model_jobs_completion_valid CHECK (((status = 'succeeded'::text) = (completed_at IS NOT NULL))),
    CONSTRAINT connector_model_jobs_digest_valid CHECK ((request_digest ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT connector_model_jobs_envelope_object CHECK ((jsonb_typeof(request_envelope_json) = 'object'::text)),
    CONSTRAINT connector_model_jobs_idempotency_valid CHECK ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key)))),
    CONSTRAINT connector_model_jobs_progress_valid CHECK (((progress >= 0) AND (progress <= 100))),
    CONSTRAINT connector_model_jobs_remote_array CHECK ((jsonb_typeof(remote_ids_json) = 'array'::text)),
    CONSTRAINT connector_model_jobs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'reconciling'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text])))
);


--
-- Name: connector_object_upload_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_object_upload_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    operation_kind text NOT NULL,
    source_asset_id integer NOT NULL,
    requested_by_user_id integer NOT NULL,
    idempotency_key text NOT NULL,
    requested_name text NOT NULL,
    reconciliation_name text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    object_key_digest text,
    object_key_envelope_json jsonb,
    notification_attempt_count integer DEFAULT 0 NOT NULL,
    reconciliation_miss_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    remote_resource_id bigint,
    uploaded_at timestamp with time zone,
    notification_attempted_at timestamp with time zone,
    reconciled_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_object_upload_jobs_attempts_valid CHECK ((((notification_attempt_count >= 0) AND (notification_attempt_count <= 2)) AND ((reconciliation_miss_count >= 0) AND (reconciliation_miss_count <= 8)))),
    CONSTRAINT connector_object_upload_jobs_completion_valid CHECK ((((status = 'succeeded'::text) = (completed_at IS NOT NULL)) AND ((status <> 'succeeded'::text) OR (remote_resource_id IS NOT NULL)))),
    CONSTRAINT connector_object_upload_jobs_digest_valid CHECK (((object_key_digest IS NULL) OR (object_key_digest ~ '^[a-f0-9]{64}$'::text))),
    CONSTRAINT connector_object_upload_jobs_envelope_object CHECK (((object_key_envelope_json IS NULL) OR (jsonb_typeof(object_key_envelope_json) = 'object'::text))),
    CONSTRAINT connector_object_upload_jobs_idempotency_valid CHECK ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key)))),
    CONSTRAINT connector_object_upload_jobs_name_valid CHECK ((((length(btrim(requested_name)) >= 1) AND (length(btrim(requested_name)) <= 200)) AND (requested_name = btrim(requested_name)) AND ((length(btrim(reconciliation_name)) >= 1) AND (length(btrim(reconciliation_name)) <= 240)) AND (reconciliation_name = btrim(reconciliation_name)))),
    CONSTRAINT connector_object_upload_jobs_operation_kind_valid CHECK ((operation_kind ~ '^[a-z][a-z0-9-]{0,63}$'::text)),
    CONSTRAINT connector_object_upload_jobs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'uploading'::text, 'notifying'::text, 'reconciling'::text, 'succeeded'::text, 'failed'::text]))),
    CONSTRAINT connector_object_upload_jobs_upload_checkpoint CHECK ((((object_key_digest IS NULL) = (object_key_envelope_json IS NULL)) AND ((uploaded_at IS NULL) = (object_key_envelope_json IS NULL))))
);


--
-- Name: connector_open_model_uploads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_open_model_uploads (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    requested_by_user_id integer NOT NULL,
    idempotency_key text NOT NULL,
    request_digest text NOT NULL,
    request_envelope_json jsonb NOT NULL,
    resource_uuid_digest text NOT NULL,
    status text DEFAULT 'requested'::text NOT NULL,
    credential_envelope_json jsonb,
    credential_expires_at timestamp with time zone,
    callback_digest text,
    callback_envelope_json jsonb,
    callback_attempt_count integer DEFAULT 0 NOT NULL,
    reconciliation_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    remote_resource_id bigint,
    asset_id integer,
    credential_issued_at timestamp with time zone,
    callback_attempted_at timestamp with time zone,
    reconciled_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_open_model_uploads_attempts_valid CHECK ((((callback_attempt_count >= 0) AND (callback_attempt_count <= 1)) AND ((reconciliation_count >= 0) AND (reconciliation_count <= 16)))),
    CONSTRAINT connector_open_model_uploads_callback_pair CHECK ((((callback_envelope_json IS NULL) OR (callback_digest IS NOT NULL)) AND ((status <> ALL (ARRAY['callback_pending'::text, 'reconciling'::text])) OR ((callback_digest IS NOT NULL) AND (callback_envelope_json IS NOT NULL))))),
    CONSTRAINT connector_open_model_uploads_completion_valid CHECK ((((status = 'succeeded'::text) = (completed_at IS NOT NULL)) AND ((status <> 'succeeded'::text) OR ((remote_resource_id IS NOT NULL) AND (asset_id IS NOT NULL))))),
    CONSTRAINT connector_open_model_uploads_digest_valid CHECK (((request_digest ~ '^[a-f0-9]{64}$'::text) AND (resource_uuid_digest ~ '^[a-f0-9]{64}$'::text) AND ((callback_digest IS NULL) OR (callback_digest ~ '^[a-f0-9]{64}$'::text)))),
    CONSTRAINT connector_open_model_uploads_envelope_object CHECK (((jsonb_typeof(request_envelope_json) = 'object'::text) AND ((credential_envelope_json IS NULL) OR (jsonb_typeof(credential_envelope_json) = 'object'::text)) AND ((callback_envelope_json IS NULL) OR (jsonb_typeof(callback_envelope_json) = 'object'::text)))),
    CONSTRAINT connector_open_model_uploads_idempotency_valid CHECK ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key)))),
    CONSTRAINT connector_open_model_uploads_status_valid CHECK ((status = ANY (ARRAY['requested'::text, 'credential_ready'::text, 'callback_pending'::text, 'reconciling'::text, 'succeeded'::text, 'expired'::text, 'failed'::text, 'blocked'::text])))
);


--
-- Name: connector_remote_resources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_remote_resources (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    resource_kind text NOT NULL,
    remote_id text NOT NULL,
    remote_version text,
    remote_updated_at timestamp with time zone,
    status text DEFAULT 'active'::text NOT NULL,
    summary_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    canonical_target_type text,
    canonical_target_id text,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    missing_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_remote_resources_canonical_pair CHECK (((canonical_target_type IS NULL) = (canonical_target_id IS NULL))),
    CONSTRAINT connector_remote_resources_kind_valid CHECK ((resource_kind = ANY (ARRAY['wayline'::text, 'flight-task'::text, 'flight-media'::text, 'flight-record'::text, 'flight-alert'::text, 'ai-alert'::text, 'map-element'::text, 'flight-area'::text, 'offline-map'::text, 'air-sense-warning'::text, 'model'::text, 'model-resource'::text, 'live-share'::text, 'stream-converter'::text, 'recording'::text, 'hms'::text, 'topology'::text, 'auto-record'::text, 'organization'::text, 'organization-user'::text, 'organization-role'::text, 'organization-permission'::text, 'project-user'::text, 'project-member'::text]))),
    CONSTRAINT connector_remote_resources_missing_time CHECK ((((status = 'missing'::text) = (missing_at IS NOT NULL)) OR (status = ANY (ARRAY['deleted'::text, 'failed'::text])))),
    CONSTRAINT connector_remote_resources_remote_id_valid CHECK ((((length(btrim(remote_id)) >= 1) AND (length(btrim(remote_id)) <= 512)) AND (remote_id = btrim(remote_id)))),
    CONSTRAINT connector_remote_resources_status_valid CHECK ((status = ANY (ARRAY['active'::text, 'missing'::text, 'deleted'::text, 'failed'::text]))),
    CONSTRAINT connector_remote_resources_summary_object CHECK ((jsonb_typeof(summary_json) = 'object'::text))
);


--
-- Name: connector_remote_resources_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.connector_remote_resources_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: connector_remote_resources_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.connector_remote_resources_id_seq OWNED BY public.connector_remote_resources.id;


--
-- Name: connector_resource_sync_states; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_resource_sync_states (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    resource_kind text NOT NULL,
    status text DEFAULT 'idle'::text NOT NULL,
    cursor_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    last_started_at timestamp with time zone,
    last_succeeded_at timestamp with time zone,
    next_attempt_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_resource_sync_states_attempt_nonnegative CHECK ((attempt_count >= 0)),
    CONSTRAINT connector_resource_sync_states_cursor_object CHECK ((jsonb_typeof(cursor_json) = 'object'::text)),
    CONSTRAINT connector_resource_sync_states_kind_valid CHECK ((resource_kind = ANY (ARRAY['inventory'::text, 'device-state'::text, 'health'::text, 'active-operations'::text, 'waylines'::text, 'flight-tasks'::text, 'flight-artifacts'::text, 'live'::text, 'geospatial'::text, 'models'::text, 'organization'::text]))),
    CONSTRAINT connector_resource_sync_states_status_valid CHECK ((status = ANY (ARRAY['idle'::text, 'running'::text, 'backoff'::text, 'failed'::text, 'disabled'::text]))),
    CONSTRAINT connector_resource_sync_states_time_valid CHECK (((last_succeeded_at IS NULL) OR (last_started_at IS NULL) OR (last_succeeded_at >= last_started_at)))
);


--
-- Name: connector_resource_sync_states_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.connector_resource_sync_states_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: connector_resource_sync_states_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.connector_resource_sync_states_id_seq OWNED BY public.connector_resource_sync_states.id;


--
-- Name: connector_sync_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_sync_runs (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    discovery_mode text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    scope_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    cursor_before_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    cursor_after_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    discovered_count integer DEFAULT 0 NOT NULL,
    managed_count integer DEFAULT 0 NOT NULL,
    missing_count integer DEFAULT 0 NOT NULL,
    error_code text,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_sync_runs_counts_nonnegative CHECK (((discovered_count >= 0) AND (managed_count >= 0) AND (missing_count >= 0))),
    CONSTRAINT connector_sync_runs_cursor_after_object CHECK ((jsonb_typeof(cursor_after_json) = 'object'::text)),
    CONSTRAINT connector_sync_runs_cursor_before_object CHECK ((jsonb_typeof(cursor_before_json) = 'object'::text)),
    CONSTRAINT connector_sync_runs_mode_valid CHECK ((discovery_mode = ANY (ARRAY['push'::text, 'poll'::text, 'subscribe'::text, 'manual-import'::text]))),
    CONSTRAINT connector_sync_runs_scope_object CHECK ((jsonb_typeof(scope_json) = 'object'::text)),
    CONSTRAINT connector_sync_runs_status_valid CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'cancelled'::text]))),
    CONSTRAINT connector_sync_runs_time_valid CHECK (((finished_at IS NULL) OR ((started_at IS NOT NULL) AND (finished_at >= started_at))))
);


--
-- Name: connector_sync_runs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.connector_sync_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: connector_sync_runs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.connector_sync_runs_id_seq OWNED BY public.connector_sync_runs.id;


--
-- Name: coordinate_references; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.coordinate_references (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    code text NOT NULL,
    name text NOT NULL,
    authority text,
    definition text,
    vertical_datum text,
    transform_version text DEFAULT '1'::text NOT NULL,
    is_project_standard boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: coordinate_references_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.coordinate_references_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: coordinate_references_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.coordinate_references_id_seq OWNED BY public.coordinate_references.id;


--
-- Name: detection_group_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.detection_group_members (
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    detection_group_id bigint NOT NULL,
    detection_id bigint NOT NULL,
    added_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: detection_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.detection_groups (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    label text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    geographic_geometry public.geometry(Polygon,4326),
    location_quality text NOT NULL,
    first_detected_at timestamp with time zone NOT NULL,
    last_detected_at timestamp with time zone NOT NULL,
    member_count integer DEFAULT 1 NOT NULL,
    aggregation_version text DEFAULT 'aerosight-detection-aggregation/v1'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT detection_groups_location_quality_valid CHECK ((location_quality = ANY (ARRAY['surveyed'::text, 'estimated'::text, 'low'::text, 'unavailable'::text]))),
    CONSTRAINT detection_groups_status_valid CHECK ((status = ANY (ARRAY['active'::text, 'superseded'::text]))),
    CONSTRAINT detection_groups_time_valid CHECK (((last_detected_at >= first_detected_at) AND (member_count > 0)))
);


--
-- Name: detection_groups_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.detection_groups_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: detection_groups_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.detection_groups_id_seq OWNED BY public.detection_groups.id;


--
-- Name: detections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.detections (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    algorithm_run_id uuid NOT NULL,
    input_asset_id integer NOT NULL,
    task_run_id integer,
    detection_key text NOT NULL,
    label text NOT NULL,
    confidence double precision NOT NULL,
    pixel_geometry_json jsonb NOT NULL,
    geographic_geometry public.geometry(Polygon,4326),
    location_quality text DEFAULT 'unavailable'::text NOT NULL,
    projection_method text DEFAULT 'image-only'::text NOT NULL,
    horizontal_error_meters double precision,
    transform_version text NOT NULL,
    attributes_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    captured_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT detections_confidence_valid CHECK (((confidence >= (0)::double precision) AND (confidence <= (1)::double precision))),
    CONSTRAINT detections_error_valid CHECK (((horizontal_error_meters IS NULL) OR (horizontal_error_meters >= (0)::double precision))),
    CONSTRAINT detections_location_quality_valid CHECK ((location_quality = ANY (ARRAY['surveyed'::text, 'estimated'::text, 'low'::text, 'unavailable'::text])))
);


--
-- Name: detections_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.detections_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: detections_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.detections_id_seq OWNED BY public.detections.id;


--
-- Name: device_adapters_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_adapters_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_adapters_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_adapters_id_seq OWNED BY public.device_adapters.id;


--
-- Name: device_capabilities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_capabilities (
    id integer NOT NULL,
    device_id integer NOT NULL,
    capability_code text NOT NULL,
    version text,
    params_schema_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    constraints_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    project_id integer NOT NULL,
    version_number integer DEFAULT 1 NOT NULL,
    declared_by_adapter_id bigint,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    device_type_id bigint NOT NULL,
    driver_definition_id bigint NOT NULL,
    availability text DEFAULT 'available'::text NOT NULL,
    availability_reason text,
    input_schema_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    output_schema_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    risk_level text DEFAULT 'low'::text NOT NULL,
    source_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT device_capabilities_availability_valid CHECK ((availability = ANY (ARRAY['available'::text, 'degraded'::text, 'unavailable'::text]))),
    CONSTRAINT device_capabilities_risk_valid CHECK ((risk_level = ANY (ARRAY['low'::text, 'medium'::text, 'high'::text, 'critical'::text])))
);


--
-- Name: device_capabilities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_capabilities_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_capabilities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_capabilities_id_seq OWNED BY public.device_capabilities.id;


--
-- Name: device_capability_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_capability_grants (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    user_id integer NOT NULL,
    scope_type text NOT NULL,
    device_type_id bigint,
    device_id integer,
    action_pattern text NOT NULL,
    effect text DEFAULT 'allow'::text NOT NULL,
    granted_by_user_id integer,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_capability_grants_action_nonempty CHECK ((length(TRIM(BOTH FROM action_pattern)) > 0)),
    CONSTRAINT device_capability_grants_effect_valid CHECK ((effect = ANY (ARRAY['allow'::text, 'deny'::text]))),
    CONSTRAINT device_capability_grants_scope_valid CHECK ((((scope_type = 'project'::text) AND (device_type_id IS NULL) AND (device_id IS NULL)) OR ((scope_type = 'device_type'::text) AND (device_type_id IS NOT NULL) AND (device_id IS NULL)) OR ((scope_type = 'device'::text) AND (device_type_id IS NULL) AND (device_id IS NOT NULL))))
);


--
-- Name: device_capability_grants_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_capability_grants_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_capability_grants_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_capability_grants_id_seq OWNED BY public.device_capability_grants.id;


--
-- Name: device_command_protocol_correlations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_command_protocol_correlations (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    command_id uuid NOT NULL,
    adapter_id bigint NOT NULL,
    mapping_version text NOT NULL,
    transaction_id text NOT NULL,
    business_id text NOT NULL,
    method text NOT NULL,
    request_topic text NOT NULL,
    request_payload_json jsonb NOT NULL,
    status text DEFAULT 'prepared'::text NOT NULL,
    reply_event_id text,
    reply_result integer,
    reply_payload_json jsonb,
    sent_at timestamp with time zone,
    replied_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_command_protocol_correlations_status_valid CHECK ((status = ANY (ARRAY['prepared'::text, 'sent'::text, 'acknowledged'::text, 'nacked'::text, 'unknown'::text])))
);


--
-- Name: device_command_protocol_correlations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_command_protocol_correlations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_command_protocol_correlations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_command_protocol_correlations_id_seq OWNED BY public.device_command_protocol_correlations.id;


--
-- Name: device_commands; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_commands (
    id uuid NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    task_run_id integer,
    task_run_step_id bigint,
    device_id integer NOT NULL,
    command_key text NOT NULL,
    idempotency_key text NOT NULL,
    capability_code text NOT NULL,
    parameters_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    safety_context_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    deadline_at timestamp with time zone NOT NULL,
    requested_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    result_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    live_stream_id bigint,
    CONSTRAINT device_commands_priority_valid CHECK (((priority >= 0) AND (priority <= 100))),
    CONSTRAINT device_commands_status_valid CHECK ((status = ANY (ARRAY['pending'::text, 'dispatchable'::text, 'sent'::text, 'acknowledged'::text, 'nacked'::text, 'timed_out'::text, 'canceled'::text, 'unknown'::text])))
);


--
-- Name: device_connections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_connections (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    adapter_id bigint NOT NULL,
    device_id integer,
    session_key text NOT NULL,
    status text DEFAULT 'unknown'::text NOT NULL,
    link_quality double precision,
    status_reason text,
    opened_at timestamp with time zone DEFAULT now() NOT NULL,
    last_heartbeat_at timestamp with time zone,
    closed_at timestamp with time zone,
    metadata_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    heartbeat_interval_seconds integer DEFAULT 30 NOT NULL,
    status_projected_at timestamp with time zone,
    CONSTRAINT device_connections_heartbeat_interval_valid CHECK (((heartbeat_interval_seconds >= 5) AND (heartbeat_interval_seconds <= 3600))),
    CONSTRAINT device_connections_status_valid CHECK ((status = ANY (ARRAY['online'::text, 'degraded'::text, 'offline'::text, 'unknown'::text])))
);


--
-- Name: device_connections_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_connections_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_connections_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_connections_id_seq OWNED BY public.device_connections.id;


--
-- Name: device_connector_bindings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_connector_bindings (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    device_id integer NOT NULL,
    connector_instance_id bigint NOT NULL,
    external_identity_id bigint NOT NULL,
    route_role text DEFAULT 'direct'::text NOT NULL,
    priority integer DEFAULT 100 NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    bound_at timestamp with time zone DEFAULT now() NOT NULL,
    unbound_at timestamp with time zone,
    metadata_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT device_connector_bindings_metadata_object CHECK ((jsonb_typeof(metadata_json) = 'object'::text)),
    CONSTRAINT device_connector_bindings_priority_nonnegative CHECK ((priority >= 0)),
    CONSTRAINT device_connector_bindings_route_role_valid CHECK ((route_role = ANY (ARRAY['direct'::text, 'gateway'::text, 'inherited'::text]))),
    CONSTRAINT device_connector_bindings_status_valid CHECK ((status = ANY (ARRAY['active'::text, 'standby'::text, 'disabled'::text, 'conflicted'::text]))),
    CONSTRAINT device_connector_bindings_time_valid CHECK (((unbound_at IS NULL) OR (unbound_at >= bound_at)))
);


--
-- Name: device_connector_bindings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_connector_bindings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_connector_bindings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_connector_bindings_id_seq OWNED BY public.device_connector_bindings.id;


--
-- Name: device_external_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_external_identities (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    adapter_id bigint NOT NULL,
    device_id integer,
    external_device_id text NOT NULL,
    external_device_type text,
    identity_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    bound_at timestamp with time zone,
    discovery_status text DEFAULT 'discovered'::text NOT NULL,
    suggested_device_type_id bigint,
    match_confidence double precision,
    source_version text,
    last_sync_run_id bigint,
    CONSTRAINT device_external_identities_match_confidence_valid CHECK (((match_confidence IS NULL) OR ((match_confidence >= (0)::double precision) AND (match_confidence <= (1)::double precision)))),
    CONSTRAINT device_external_identities_status_valid CHECK ((discovery_status = ANY (ARRAY['discovered'::text, 'managed'::text, 'ignored'::text, 'conflicted'::text, 'missing'::text])))
);


--
-- Name: device_external_identities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_external_identities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_external_identities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_external_identities_id_seq OWNED BY public.device_external_identities.id;


--
-- Name: device_latest_telemetry; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_latest_telemetry (
    device_id integer NOT NULL,
    project_id integer NOT NULL,
    adapter_id bigint NOT NULL,
    event_id text NOT NULL,
    telemetry_type text NOT NULL,
    sequence_number bigint,
    captured_at timestamp with time zone NOT NULL,
    received_at timestamp with time zone NOT NULL,
    payload_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    quality_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: device_network_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_network_profiles (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    name text NOT NULL,
    mode text NOT NULL,
    mqtt_endpoint text,
    api_public_base_url text,
    websocket_public_url text,
    media_ingest_base_url text,
    media_playback_base_url text,
    tls_required boolean DEFAULT false NOT NULL,
    secret_ref text,
    status text DEFAULT 'unverified'::text NOT NULL,
    config_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    last_validation_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    last_validated_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_network_profiles_mode_valid CHECK ((mode = ANY (ARRAY['lan'::text, 'public'::text]))),
    CONSTRAINT device_network_profiles_public_tls CHECK (((mode <> 'public'::text) OR tls_required)),
    CONSTRAINT device_network_profiles_status_valid CHECK ((status = ANY (ARRAY['unverified'::text, 'valid'::text, 'invalid'::text, 'degraded'::text])))
);


--
-- Name: device_network_profiles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_network_profiles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_network_profiles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_network_profiles_id_seq OWNED BY public.device_network_profiles.id;


--
-- Name: device_protocol_cursors; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_protocol_cursors (
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    adapter_id bigint NOT NULL,
    route_key text NOT NULL,
    last_timestamp_ms bigint NOT NULL,
    last_transaction_id text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: device_protocol_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_protocol_messages (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    adapter_id bigint NOT NULL,
    gateway_sn text NOT NULL,
    device_sn text NOT NULL,
    topic text NOT NULL,
    route_kind text NOT NULL,
    transaction_id text NOT NULL,
    business_id text,
    method text,
    timestamp_ms bigint NOT NULL,
    sequence_number bigint,
    qos smallint NOT NULL,
    duplicate_flag boolean DEFAULT false NOT NULL,
    payload_json jsonb NOT NULL,
    disposition text DEFAULT 'accepted'::text NOT NULL,
    disposition_reason text,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_protocol_messages_disposition_valid CHECK ((disposition = ANY (ARRAY['accepted'::text, 'out_of_order'::text]))),
    CONSTRAINT device_protocol_messages_route_valid CHECK ((route_kind = ANY (ARRAY['topology'::text, 'state'::text, 'telemetry'::text, 'event'::text, 'request'::text, 'service_reply'::text])))
);


--
-- Name: device_protocol_messages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_protocol_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_protocol_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_protocol_messages_id_seq OWNED BY public.device_protocol_messages.id;


--
-- Name: device_relationships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_relationships (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    from_device_id integer NOT NULL,
    to_device_id integer NOT NULL,
    relation_type text NOT NULL,
    source_type text DEFAULT 'manual'::text NOT NULL,
    valid_from timestamp with time zone DEFAULT now() NOT NULL,
    valid_until timestamp with time zone,
    metadata_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_relationships_not_self CHECK ((from_device_id <> to_device_id)),
    CONSTRAINT device_relationships_source_valid CHECK ((source_type = ANY (ARRAY['driver'::text, 'discovery'::text, 'manual'::text, 'migration'::text]))),
    CONSTRAINT device_relationships_valid_range CHECK (((valid_until IS NULL) OR (valid_until > valid_from)))
);


--
-- Name: device_relationships_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_relationships_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_relationships_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_relationships_id_seq OWNED BY public.device_relationships.id;


--
-- Name: device_stream_channels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_stream_channels (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    device_id integer NOT NULL,
    capability_code text NOT NULL,
    channel_key text NOT NULL,
    display_name text NOT NULL,
    data_type text NOT NULL,
    schema_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    unit text,
    protocol text,
    quality_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    availability text DEFAULT 'available'::text NOT NULL,
    availability_reason text,
    source_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    stable_channel_id text NOT NULL,
    CONSTRAINT device_stream_channels_availability_valid CHECK ((availability = ANY (ARRAY['available'::text, 'degraded'::text, 'unavailable'::text]))),
    CONSTRAINT device_stream_channels_data_type_valid CHECK ((data_type = ANY (ARRAY['video'::text, 'audio'::text, 'telemetry'::text, 'sensor'::text, 'events'::text])))
);


--
-- Name: device_stream_channels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_stream_channels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_stream_channels_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_stream_channels_id_seq OWNED BY public.device_stream_channels.id;


--
-- Name: device_telemetry; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_telemetry (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    adapter_id bigint NOT NULL,
    device_id integer NOT NULL,
    event_id text NOT NULL,
    telemetry_type text NOT NULL,
    sequence_number bigint,
    captured_at timestamp with time zone NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    payload_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    quality_json jsonb DEFAULT '{}'::jsonb NOT NULL
)
PARTITION BY RANGE (captured_at);


--
-- Name: device_telemetry_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_telemetry_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_telemetry_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_telemetry_id_seq OWNED BY public.device_telemetry.id;


--
-- Name: device_telemetry_default; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_telemetry_default (
    id bigint DEFAULT nextval('public.device_telemetry_id_seq'::regclass) NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    adapter_id bigint NOT NULL,
    device_id integer NOT NULL,
    event_id text NOT NULL,
    telemetry_type text NOT NULL,
    sequence_number bigint,
    captured_at timestamp with time zone NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    payload_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    quality_json jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: device_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_types (
    id bigint NOT NULL,
    type_key text NOT NULL,
    version integer NOT NULL,
    display_name text NOT NULL,
    category text NOT NULL,
    vendor text,
    model text,
    driver_definition_id bigint NOT NULL,
    driver_version_constraint text DEFAULT '*'::text NOT NULL,
    capability_profile_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_types_capability_profile_object CHECK ((jsonb_typeof(capability_profile_json) = 'object'::text)),
    CONSTRAINT device_types_status_valid CHECK ((status = ANY (ARRAY['active'::text, 'retired'::text]))),
    CONSTRAINT device_types_version_positive CHECK ((version > 0))
);


--
-- Name: device_types_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.device_types_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: device_types_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.device_types_id_seq OWNED BY public.device_types.id;


--
-- Name: devices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.devices (
    id integer NOT NULL,
    project_id integer NOT NULL,
    name text NOT NULL,
    type text NOT NULL,
    status text DEFAULT 'offline'::text NOT NULL,
    last_seen_at timestamp without time zone,
    config_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    metadata_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    adapter_id bigint,
    device_model text,
    firmware_version text,
    status_reason text,
    uav_registration_number text,
    registration_valid_until timestamp with time zone,
    remote_identification_code text,
    responsible_user_id integer,
    device_type_id bigint NOT NULL,
    status_observed_at timestamp with time zone,
    status_projected_at timestamp with time zone DEFAULT now() NOT NULL,
    data_freshness text DEFAULT 'unknown'::text NOT NULL,
    raw_status_ref text,
    CONSTRAINT devices_connectivity_status_valid CHECK ((status = ANY (ARRAY['online'::text, 'degraded'::text, 'offline'::text, 'unknown'::text, 'unavailable'::text]))),
    CONSTRAINT devices_data_freshness_valid CHECK ((data_freshness = ANY (ARRAY['fresh'::text, 'stale'::text, 'expired'::text, 'unknown'::text])))
);


--
-- Name: devices_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.devices_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: devices_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.devices_id_seq OWNED BY public.devices.id;


--
-- Name: driver_definitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.driver_definitions (
    id bigint NOT NULL,
    driver_key text NOT NULL,
    version text NOT NULL,
    display_name text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    manifest_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT driver_definitions_manifest_object CHECK ((jsonb_typeof(manifest_json) = 'object'::text)),
    CONSTRAINT driver_definitions_status_valid CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text, 'retired'::text])))
);


--
-- Name: driver_definitions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.driver_definitions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: driver_definitions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.driver_definitions_id_seq OWNED BY public.driver_definitions.id;


--
-- Name: event_feedback; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.event_feedback (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    perception_event_id uuid NOT NULL,
    action text NOT NULL,
    value_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    reason text NOT NULL,
    actor_user_id integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT event_feedback_action_valid CHECK ((action = ANY (ARRAY['confirm'::text, 'false_positive'::text, 'category_correction'::text, 'assign'::text, 'acknowledge'::text, 'investigate'::text, 'dismiss'::text, 'resolve'::text])))
);


--
-- Name: event_feedback_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.event_feedback_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: event_feedback_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.event_feedback_id_seq OWNED BY public.event_feedback.id;


--
-- Name: event_rule_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.event_rule_versions (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    event_rule_id bigint NOT NULL,
    version integer NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    label text NOT NULL,
    minimum_confidence double precision NOT NULL,
    severity text NOT NULL,
    deduplication_window_seconds integer DEFAULT 3600 NOT NULL,
    conditions_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by_user_id integer,
    published_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    CONSTRAINT event_rule_versions_confidence_valid CHECK (((minimum_confidence >= (0)::double precision) AND (minimum_confidence <= (1)::double precision))),
    CONSTRAINT event_rule_versions_severity_valid CHECK ((severity = ANY (ARRAY['low'::text, 'medium'::text, 'high'::text, 'critical'::text]))),
    CONSTRAINT event_rule_versions_status_valid CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text, 'retired'::text]))),
    CONSTRAINT event_rule_versions_window_valid CHECK ((deduplication_window_seconds > 0))
);


--
-- Name: event_rule_versions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.event_rule_versions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: event_rule_versions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.event_rule_versions_id_seq OWNED BY public.event_rule_versions.id;


--
-- Name: event_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.event_rules (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    name text NOT NULL,
    status text DEFAULT 'disabled'::text NOT NULL,
    current_published_version_id bigint,
    created_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT event_rules_status_valid CHECK ((status = ANY (ARRAY['disabled'::text, 'active'::text, 'retired'::text])))
);


--
-- Name: event_rules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.event_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: event_rules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.event_rules_id_seq OWNED BY public.event_rules.id;


--
-- Name: evidence_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.evidence_links (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    target_type text NOT NULL,
    target_id text NOT NULL,
    asset_id integer NOT NULL,
    asset_version integer NOT NULL,
    asset_checksum_sha256 text NOT NULL,
    start_offset_ms bigint,
    end_offset_ms bigint,
    is_published boolean DEFAULT false NOT NULL,
    created_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT evidence_links_checksum_valid CHECK ((asset_checksum_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT evidence_links_offsets_valid CHECK ((((start_offset_ms IS NULL) AND (end_offset_ms IS NULL)) OR ((start_offset_ms IS NOT NULL) AND (start_offset_ms >= 0) AND (end_offset_ms IS NOT NULL) AND (end_offset_ms > start_offset_ms)))),
    CONSTRAINT evidence_links_target_type_valid CHECK ((target_type = ANY (ARRAY['detection'::text, 'track'::text, 'event'::text, 'report'::text, 'issue'::text, 'task_run'::text]))),
    CONSTRAINT evidence_links_version_positive CHECK ((asset_version > 0))
);


--
-- Name: evidence_links_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.evidence_links_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: evidence_links_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.evidence_links_id_seq OWNED BY public.evidence_links.id;


--
-- Name: generated_report_evidence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.generated_report_evidence (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    report_version_id uuid NOT NULL,
    evidence_type text NOT NULL,
    evidence_id text NOT NULL,
    evidence_version text NOT NULL,
    asset_id integer,
    checksum_sha256 text,
    href text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT generated_report_evidence_type_valid CHECK ((evidence_type = ANY (ARRAY['task_run'::text, 'task_version'::text, 'device'::text, 'track'::text, 'step'::text, 'event'::text, 'feedback'::text, 'asset'::text])))
);


--
-- Name: generated_report_evidence_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.generated_report_evidence_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: generated_report_evidence_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.generated_report_evidence_id_seq OWNED BY public.generated_report_evidence.id;


--
-- Name: generated_report_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.generated_report_versions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    generated_report_id uuid NOT NULL,
    version integer NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    completeness text NOT NULL,
    content_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    data_gaps_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_by_user_id integer,
    published_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    CONSTRAINT generated_report_versions_completeness_valid CHECK ((completeness = ANY (ARRAY['complete'::text, 'incomplete'::text, 'failed'::text]))),
    CONSTRAINT generated_report_versions_status_valid CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text, 'retired'::text]))),
    CONSTRAINT generated_report_versions_version_positive CHECK ((version > 0))
);


--
-- Name: generated_reports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.generated_reports (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    source_type text NOT NULL,
    source_id text NOT NULL,
    title text NOT NULL,
    current_published_version_id uuid,
    created_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT generated_reports_source_type_valid CHECK ((source_type = ANY (ARRAY['task_run'::text, 'perception_event'::text])))
);


--
-- Name: idempotency_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.idempotency_records (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    actor_key text NOT NULL,
    operation text NOT NULL,
    idempotency_key text NOT NULL,
    request_hash text NOT NULL,
    status text DEFAULT 'processing'::text NOT NULL,
    response_json jsonb,
    error_code text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    expires_at timestamp with time zone DEFAULT (now() + '24:00:00'::interval) NOT NULL,
    CONSTRAINT idempotency_records_status_valid CHECK ((status = ANY (ARRAY['processing'::text, 'completed'::text, 'failed'::text])))
);


--
-- Name: idempotency_records_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.idempotency_records_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: idempotency_records_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.idempotency_records_id_seq OWNED BY public.idempotency_records.id;


--
-- Name: issue_assignees; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.issue_assignees (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    issue_id integer NOT NULL,
    assignee_type text NOT NULL,
    user_id integer,
    agent_id integer,
    assigned_by_user_id integer NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    removed_at timestamp with time zone,
    CONSTRAINT issue_assignees_active_time_valid CHECK ((active = (removed_at IS NULL))),
    CONSTRAINT issue_assignees_subject_valid CHECK ((((assignee_type = 'user'::text) AND (user_id IS NOT NULL) AND (agent_id IS NULL)) OR ((assignee_type = 'agent'::text) AND (agent_id IS NOT NULL) AND (user_id IS NULL)))),
    CONSTRAINT issue_assignees_type_valid CHECK ((assignee_type = ANY (ARRAY['user'::text, 'agent'::text])))
);


--
-- Name: issue_assignees_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.issue_assignees_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: issue_assignees_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.issue_assignees_id_seq OWNED BY public.issue_assignees.id;


--
-- Name: issue_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.issue_events (
    id integer NOT NULL,
    project_id integer NOT NULL,
    issue_id integer NOT NULL,
    event_type text NOT NULL,
    body text,
    metadata_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    actor_user_id integer,
    actor_agent_id integer,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    client_key text
);


--
-- Name: issue_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.issue_events_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: issue_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.issue_events_id_seq OWNED BY public.issue_events.id;


--
-- Name: issue_feedback; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.issue_feedback (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    issue_id integer NOT NULL,
    detection_id bigint NOT NULL,
    algorithm_definition_version_id bigint NOT NULL,
    task_version_id bigint,
    task_run_step_id bigint,
    action text NOT NULL,
    corrected_label text,
    disposition text,
    reason text NOT NULL,
    client_key uuid NOT NULL,
    evidence_snapshot_json jsonb NOT NULL,
    actor_user_id integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT issue_feedback_action_valid CHECK ((action = ANY (ARRAY['confirm'::text, 'false_positive'::text, 'category_correction'::text, 'disposition'::text]))),
    CONSTRAINT issue_feedback_correction_valid CHECK (((action = 'category_correction'::text) = (corrected_label IS NOT NULL))),
    CONSTRAINT issue_feedback_disposition_valid CHECK (((disposition IS NULL) OR (disposition = ANY (ARRAY['resolved'::text, 'monitoring'::text, 'remediated'::text, 'accepted_risk'::text, 'not_applicable'::text]))))
);


--
-- Name: issue_feedback_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.issue_feedback_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: issue_feedback_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.issue_feedback_id_seq OWNED BY public.issue_feedback.id;


--
-- Name: issue_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.issue_links (
    id integer NOT NULL,
    project_id integer NOT NULL,
    issue_id integer NOT NULL,
    link_type text NOT NULL,
    target_id text NOT NULL,
    created_by_user_id integer,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: issue_links_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.issue_links_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: issue_links_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.issue_links_id_seq OWNED BY public.issue_links.id;


--
-- Name: issues; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.issues (
    id integer NOT NULL,
    project_id integer NOT NULL,
    number integer NOT NULL,
    title text NOT NULL,
    description text,
    source_type text NOT NULL,
    source_id integer,
    status text DEFAULT 'open'::text NOT NULL,
    priority text DEFAULT 'medium'::text NOT NULL,
    task_run_id integer,
    opened_by_user_id integer,
    assignee_user_id integer,
    closed_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    task_version_id bigint,
    condition_scope_key text,
    business_object_key text,
    occurrence_count integer DEFAULT 1 NOT NULL,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    labels_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    state_version integer DEFAULT 0 NOT NULL,
    CONSTRAINT issues_labels_array CHECK ((jsonb_typeof(labels_json) = 'array'::text)),
    CONSTRAINT issues_occurrence_positive CHECK ((occurrence_count > 0)),
    CONSTRAINT issues_state_version_nonnegative CHECK ((state_version >= 0))
);


--
-- Name: issues_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.issues_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: issues_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.issues_id_seq OWNED BY public.issues.id;


--
-- Name: live_streams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.live_streams (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    device_id integer NOT NULL,
    task_run_id integer,
    adapter_id bigint,
    stream_key text NOT NULL,
    source_type text NOT NULL,
    status text DEFAULT 'starting'::text NOT NULL,
    playback_ref text,
    playback_locator_expires_at timestamp with time zone,
    status_reason text,
    started_by_user_id integer,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    last_active_at timestamp with time zone,
    ended_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    stream_channel_id bigint,
    ingest_ref text,
    lease_expires_at timestamp with time zone,
    session_token uuid DEFAULT gen_random_uuid() NOT NULL,
    lease_owner text,
    vendor_stream_ref text,
    supplier text,
    supplier_protocol text,
    supplier_adapter_version text,
    supplier_reference_digest text,
    supplier_credential_expires_at timestamp with time zone,
    supplier_credential_envelope_json jsonb,
    start_attempted_at timestamp with time zone,
    start_accepted_at timestamp with time zone,
    last_playback_at timestamp with time zone,
    local_authorization_revoked_at timestamp with time zone,
    remote_evidence_at timestamp with time zone,
    CONSTRAINT live_streams_flighthub_adapter_required CHECK (((source_type <> 'dji_flighthub'::text) OR (adapter_id IS NOT NULL))),
    CONSTRAINT live_streams_lease_complete CHECK ((((lease_owner IS NULL) AND (lease_expires_at IS NULL)) OR ((lease_owner IS NOT NULL) AND (lease_expires_at IS NOT NULL)))),
    CONSTRAINT live_streams_status_valid CHECK ((status = ANY (ARRAY['requested'::text, 'starting'::text, 'live'::text, 'degraded'::text, 'failed'::text, 'stopping'::text, 'stopped'::text]))),
    CONSTRAINT live_streams_supplier_credential_complete CHECK (((supplier_credential_envelope_json IS NULL) OR ((source_type = 'dji_flighthub'::text) AND (supplier IS NOT NULL) AND (supplier_protocol IS NOT NULL) AND (supplier_adapter_version IS NOT NULL) AND (supplier_reference_digest IS NOT NULL) AND (supplier_credential_expires_at IS NOT NULL) AND (start_accepted_at IS NOT NULL) AND (local_authorization_revoked_at IS NULL)))),
    CONSTRAINT live_streams_supplier_digest_valid CHECK (((supplier_reference_digest IS NULL) OR (supplier_reference_digest ~ '^[a-f0-9]{64}$'::text))),
    CONSTRAINT live_streams_supplier_envelope_object CHECK (((supplier_credential_envelope_json IS NULL) OR (jsonb_typeof(supplier_credential_envelope_json) = 'object'::text)))
);


--
-- Name: live_streams_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.live_streams_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: live_streams_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.live_streams_id_seq OWNED BY public.live_streams.id;


--
-- Name: observations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.observations (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    adapter_id bigint NOT NULL,
    device_id integer NOT NULL,
    calibration_id bigint,
    observation_type text NOT NULL,
    source_event_id text NOT NULL,
    captured_at timestamp with time zone NOT NULL,
    received_at timestamp with time zone NOT NULL,
    time_quality text DEFAULT 'trusted'::text NOT NULL,
    original_crs_id bigint,
    original_geometry public.geometry(GeometryZ),
    standard_geometry public.geometry(GeometryZ,4326),
    properties_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    quality_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    validity text DEFAULT 'valid'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    task_run_id integer,
    CONSTRAINT observations_time_quality_valid CHECK ((time_quality = ANY (ARRAY['trusted'::text, 'uncertain'::text, 'invalid'::text]))),
    CONSTRAINT observations_validity_valid CHECK ((validity = ANY (ARRAY['valid'::text, 'degraded'::text, 'late'::text, 'invalid'::text])))
);


--
-- Name: observations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.observations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: observations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.observations_id_seq OWNED BY public.observations.id;


--
-- Name: outbox_consumptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.outbox_consumptions (
    consumer_name text NOT NULL,
    event_id text NOT NULL,
    consumed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: outbox_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.outbox_events (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    event_id text NOT NULL,
    event_type text NOT NULL,
    aggregate_type text,
    aggregate_id text,
    payload_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    max_attempts integer DEFAULT 8 NOT NULL,
    available_at timestamp with time zone DEFAULT now() NOT NULL,
    locked_by text,
    locked_until timestamp with time zone,
    last_error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT outbox_events_attempts_valid CHECK (((attempts >= 0) AND (max_attempts > 0))),
    CONSTRAINT outbox_events_status_valid CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'completed'::text, 'dead'::text])))
);


--
-- Name: outbox_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.outbox_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: outbox_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.outbox_events_id_seq OWNED BY public.outbox_events.id;


--
-- Name: perception_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.perception_events (
    id uuid NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    event_rule_version_id bigint NOT NULL,
    detection_group_id bigint NOT NULL,
    deduplication_key text NOT NULL,
    title text DEFAULT '疑似违建'::text NOT NULL,
    severity text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    occurrence_count integer DEFAULT 1 NOT NULL,
    state_version integer DEFAULT 0 NOT NULL,
    assigned_user_id integer,
    first_detected_at timestamp with time zone NOT NULL,
    last_detected_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    resolved_at timestamp with time zone,
    CONSTRAINT perception_events_counts_valid CHECK (((occurrence_count > 0) AND (state_version >= 0) AND (last_detected_at >= first_detected_at))),
    CONSTRAINT perception_events_severity_valid CHECK ((severity = ANY (ARRAY['low'::text, 'medium'::text, 'high'::text, 'critical'::text]))),
    CONSTRAINT perception_events_status_valid CHECK ((status = ANY (ARRAY['open'::text, 'acknowledged'::text, 'investigating'::text, 'resolved'::text, 'dismissed'::text])))
);


--
-- Name: platform_audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_audit_events (
    id bigint NOT NULL,
    actor_user_id integer NOT NULL,
    request_id text NOT NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    input_hash text NOT NULL,
    result_hash text,
    status text DEFAULT 'accepted'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT platform_audit_events_status_valid CHECK ((status = ANY (ARRAY['accepted'::text, 'completed'::text])))
);


--
-- Name: platform_audit_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.platform_audit_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: platform_audit_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.platform_audit_events_id_seq OWNED BY public.platform_audit_events.id;


--
-- Name: poses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.poses (
    observation_id bigint NOT NULL,
    project_id integer NOT NULL,
    device_id integer NOT NULL,
    captured_at timestamp with time zone NOT NULL,
    standard_position public.geometry(PointZ,4326),
    original_position public.geometry(PointZ),
    orientation_x double precision,
    orientation_y double precision,
    orientation_z double precision,
    orientation_w double precision,
    velocity_x double precision,
    velocity_y double precision,
    velocity_z double precision,
    horizontal_accuracy_m double precision,
    vertical_accuracy_m double precision,
    attitude_accuracy_deg double precision,
    vertical_datum text,
    transform_version text,
    spatial_quality text DEFAULT 'usable'::text NOT NULL,
    CONSTRAINT poses_accuracy_nonnegative CHECK ((((horizontal_accuracy_m IS NULL) OR (horizontal_accuracy_m >= (0)::double precision)) AND ((vertical_accuracy_m IS NULL) OR (vertical_accuracy_m >= (0)::double precision)) AND ((attitude_accuracy_deg IS NULL) OR (attitude_accuracy_deg >= (0)::double precision)))),
    CONSTRAINT poses_spatial_quality_valid CHECK ((spatial_quality = ANY (ARRAY['usable'::text, 'degraded'::text, 'unusable'::text])))
);


--
-- Name: project_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_events (
    cursor bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    event_id text NOT NULL,
    event_type text NOT NULL,
    payload_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: project_events_cursor_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.project_events_cursor_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: project_events_cursor_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.project_events_cursor_seq OWNED BY public.project_events.cursor;


--
-- Name: project_feature_flags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_feature_flags (
    project_id integer NOT NULL,
    device_commands_enabled boolean DEFAULT false NOT NULL,
    operations_overview_enabled boolean DEFAULT false NOT NULL,
    object_storage_enabled boolean DEFAULT false NOT NULL,
    external_algorithms_enabled boolean DEFAULT false NOT NULL,
    automatic_ai_enabled boolean DEFAULT false NOT NULL,
    dependency_health_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_by_user_id integer,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    flighthub_action_flags_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT project_feature_flags_flighthub_actions_object CHECK ((jsonb_typeof(flighthub_action_flags_json) = 'object'::text))
);


--
-- Name: project_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_permissions (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    user_id integer NOT NULL,
    permission text NOT NULL,
    granted_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: project_permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.project_permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: project_permissions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.project_permissions_id_seq OWNED BY public.project_permissions.id;


--
-- Name: projects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.projects (
    id integer NOT NULL,
    team_id integer NOT NULL,
    name text NOT NULL,
    description text,
    created_by_user_id integer,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    current_safety_policy_version_id bigint
);


--
-- Name: projects_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.projects_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: projects_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.projects_id_seq OWNED BY public.projects.id;


--
-- Name: retention_cleanup_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.retention_cleanup_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    retention_policy_id bigint NOT NULL,
    mode text DEFAULT 'dry_run'::text NOT NULL,
    status text DEFAULT 'planned'::text NOT NULL,
    plan_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    candidate_count integer DEFAULT 0 NOT NULL,
    deleted_count integer DEFAULT 0 NOT NULL,
    created_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    error_code text,
    CONSTRAINT retention_cleanup_runs_counts_valid CHECK (((candidate_count >= 0) AND (deleted_count >= 0) AND (deleted_count <= candidate_count))),
    CONSTRAINT retention_cleanup_runs_mode_valid CHECK ((mode = ANY (ARRAY['dry_run'::text, 'execute'::text]))),
    CONSTRAINT retention_cleanup_runs_status_valid CHECK ((status = ANY (ARRAY['planned'::text, 'running'::text, 'completed'::text, 'failed'::text])))
);


--
-- Name: retention_deletion_tombstones; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.retention_deletion_tombstones (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    cleanup_run_id uuid NOT NULL,
    retention_policy_id bigint NOT NULL,
    asset_id integer NOT NULL,
    storage_key_hash text NOT NULL,
    checksum_sha256 text,
    reason_code text NOT NULL,
    deleted_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT retention_tombstones_checksum_valid CHECK (((checksum_sha256 IS NULL) OR (checksum_sha256 ~ '^[a-f0-9]{64}$'::text))),
    CONSTRAINT retention_tombstones_storage_hash_valid CHECK ((storage_key_hash ~ '^[a-f0-9]{64}$'::text))
);


--
-- Name: retention_holds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.retention_holds (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    asset_id integer NOT NULL,
    reason text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    hold_until timestamp with time zone,
    created_by_user_id integer,
    released_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    released_at timestamp with time zone,
    CONSTRAINT retention_holds_reason_present CHECK ((length(TRIM(BOTH FROM reason)) > 0)),
    CONSTRAINT retention_holds_release_complete CHECK ((((status = 'active'::text) AND (released_at IS NULL)) OR ((status = 'released'::text) AND (released_at IS NOT NULL)))),
    CONSTRAINT retention_holds_status_valid CHECK ((status = ANY (ARRAY['active'::text, 'released'::text])))
);


--
-- Name: retention_policies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.retention_policies (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    policy_key text NOT NULL,
    version integer NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    retention_days integer NOT NULL,
    derivative_retention_days integer NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_by_user_id integer,
    published_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    CONSTRAINT retention_policies_duration_valid CHECK (((retention_days > 0) AND (derivative_retention_days > 0))),
    CONSTRAINT retention_policies_status_valid CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text, 'retired'::text]))),
    CONSTRAINT retention_policies_version_valid CHECK ((version > 0))
);


--
-- Name: retention_policies_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.retention_policies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: retention_policies_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.retention_policies_id_seq OWNED BY public.retention_policies.id;


--
-- Name: safety_policy_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.safety_policy_versions (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    version integer NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    project_boundary public.geometry(Polygon,4326),
    restricted_areas public.geometry(MultiPolygon,4326),
    max_altitude_meters double precision NOT NULL,
    max_speed_meters_per_second double precision NOT NULL,
    minimum_battery_percent double precision NOT NULL,
    allowed_windows_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    required_compliance_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    optional_compliance_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    exemptions_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_by_user_id integer,
    published_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    CONSTRAINT safety_policy_versions_limits_valid CHECK (((max_altitude_meters > (0)::double precision) AND (max_speed_meters_per_second > (0)::double precision) AND ((minimum_battery_percent >= (0)::double precision) AND (minimum_battery_percent <= (100)::double precision)))),
    CONSTRAINT safety_policy_versions_status_valid CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text])))
);


--
-- Name: safety_policy_versions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.safety_policy_versions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: safety_policy_versions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.safety_policy_versions_id_seq OWNED BY public.safety_policy_versions.id;


--
-- Name: sensor_calibrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sensor_calibrations (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    device_id integer NOT NULL,
    sensor_key text NOT NULL,
    version integer NOT NULL,
    intrinsic_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    extrinsic_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    quality_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    valid_from timestamp with time zone NOT NULL,
    valid_until timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sensor_calibrations_valid_range CHECK (((valid_until IS NULL) OR (valid_until > valid_from)))
);


--
-- Name: sensor_calibrations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sensor_calibrations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sensor_calibrations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sensor_calibrations_id_seq OWNED BY public.sensor_calibrations.id;


--
-- Name: sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sessions (
    token text NOT NULL,
    data bytea NOT NULL,
    expiry timestamp with time zone NOT NULL
);


--
-- Name: task_run_steps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.task_run_steps (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    task_run_id integer NOT NULL,
    task_step_id bigint NOT NULL,
    "position" integer NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    result_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    input_snapshot_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    output_snapshot_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    condition_result_json jsonb,
    execution_key text,
    CONSTRAINT task_run_steps_condition_object CHECK (((condition_result_json IS NULL) OR (jsonb_typeof(condition_result_json) = 'object'::text))),
    CONSTRAINT task_run_steps_input_object CHECK ((jsonb_typeof(input_snapshot_json) = 'object'::text)),
    CONSTRAINT task_run_steps_output_object CHECK ((jsonb_typeof(output_snapshot_json) = 'object'::text)),
    CONSTRAINT task_run_steps_position_valid CHECK ((("position" > 0) AND (attempt_count >= 0))),
    CONSTRAINT task_run_steps_status_valid CHECK ((status = ANY (ARRAY['pending'::text, 'dispatching'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'skipped'::text, 'paused'::text])))
);


--
-- Name: task_run_steps_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.task_run_steps_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: task_run_steps_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.task_run_steps_id_seq OWNED BY public.task_run_steps.id;


--
-- Name: task_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.task_runs (
    id integer NOT NULL,
    project_id integer NOT NULL,
    task_id integer NOT NULL,
    trigger_source text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    input_snapshot_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    output_snapshot_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    error_message text,
    started_at timestamp without time zone,
    finished_at timestamp without time zone,
    created_by_user_id integer,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    task_version_id bigint,
    team_id integer NOT NULL,
    selected_device_id integer,
    safety_policy_version_id bigint,
    approval_request_id uuid,
    state_version integer DEFAULT 0 NOT NULL,
    current_step_position integer,
    preflight_snapshot_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    state_reason text,
    operation_approval_reference text,
    operation_approval_valid_until timestamp with time zone,
    takeoff_confirmed_at timestamp with time zone,
    takeoff_confirmed_by_user_id integer,
    responsible_user_id integer,
    incident_report_reference text,
    incident_reported_at timestamp with time zone,
    trigger_key text,
    CONSTRAINT task_runs_current_step_valid CHECK (((current_step_position IS NULL) OR (current_step_position > 0))),
    CONSTRAINT task_runs_incident_report_complete CHECK (((incident_report_reference IS NULL) = (incident_reported_at IS NULL))),
    CONSTRAINT task_runs_state_version_valid CHECK ((state_version >= 0)),
    CONSTRAINT task_runs_status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'blocked'::text, 'ready'::text, 'dispatching'::text, 'running'::text, 'paused'::text, 'succeeded'::text, 'failed'::text, 'canceling'::text, 'canceled'::text]))),
    CONSTRAINT task_runs_takeoff_confirmation_complete CHECK (((takeoff_confirmed_at IS NULL) = (takeoff_confirmed_by_user_id IS NULL))),
    CONSTRAINT task_runs_trigger_key_normalized CHECK (((trigger_key IS NULL) OR (((length(trigger_key) >= 3) AND (length(trigger_key) <= 512)) AND (trigger_key = btrim(trigger_key)))))
);


--
-- Name: task_runs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.task_runs_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: task_runs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.task_runs_id_seq OWNED BY public.task_runs.id;


--
-- Name: task_steps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.task_steps (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    task_version_id bigint NOT NULL,
    "position" integer NOT NULL,
    step_key text NOT NULL,
    name text NOT NULL,
    capability_code text,
    action text NOT NULL,
    parameters_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    failure_policy_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    media_requirements_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    uses text DEFAULT 'device.command'::text NOT NULL,
    input_schema_json jsonb DEFAULT '{"type": "object", "properties": {}}'::jsonb NOT NULL,
    output_schema_json jsonb DEFAULT '{"type": "object", "properties": {}}'::jsonb NOT NULL,
    condition_json jsonb,
    depends_on_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    timeout_seconds integer DEFAULT 300 NOT NULL,
    retry_policy_json jsonb DEFAULT '{"maxAttempts": 1, "backoffSeconds": 0}'::jsonb NOT NULL,
    CONSTRAINT task_steps_condition_object CHECK (((condition_json IS NULL) OR (jsonb_typeof(condition_json) = 'object'::text))),
    CONSTRAINT task_steps_depends_array CHECK ((jsonb_typeof(depends_on_json) = 'array'::text)),
    CONSTRAINT task_steps_input_schema_object CHECK ((jsonb_typeof(input_schema_json) = 'object'::text)),
    CONSTRAINT task_steps_output_schema_object CHECK ((jsonb_typeof(output_schema_json) = 'object'::text)),
    CONSTRAINT task_steps_position_positive CHECK (("position" > 0)),
    CONSTRAINT task_steps_retry_policy_object CHECK ((jsonb_typeof(retry_policy_json) = 'object'::text)),
    CONSTRAINT task_steps_timeout_positive CHECK (((timeout_seconds > 0) AND (timeout_seconds <= 86400))),
    CONSTRAINT task_steps_uses_valid CHECK ((uses = ANY (ARRAY['device.command'::text, 'device.collect'::text, 'algorithm.run'::text, 'issue.create-or-update'::text, 'copilot.run'::text, 'report.generate'::text])))
);


--
-- Name: task_steps_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.task_steps_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: task_steps_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.task_steps_id_seq OWNED BY public.task_steps.id;


--
-- Name: task_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.task_versions (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    team_id integer NOT NULL,
    task_id integer NOT NULL,
    version integer NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    definition_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    script text NOT NULL,
    created_by_user_id integer,
    published_by_user_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    input_schema_json jsonb DEFAULT '{"type": "object", "properties": {}, "additionalProperties": false}'::jsonb NOT NULL,
    trigger_json jsonb DEFAULT '{"type": "manual"}'::jsonb NOT NULL,
    concurrency_limit integer DEFAULT 1 NOT NULL,
    CONSTRAINT task_versions_concurrency_positive CHECK (((concurrency_limit > 0) AND (concurrency_limit <= 100))),
    CONSTRAINT task_versions_input_schema_object CHECK ((jsonb_typeof(input_schema_json) = 'object'::text)),
    CONSTRAINT task_versions_status_valid CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text, 'retired'::text]))),
    CONSTRAINT task_versions_trigger_object CHECK ((jsonb_typeof(trigger_json) = 'object'::text)),
    CONSTRAINT task_versions_version_positive CHECK ((version > 0))
);


--
-- Name: task_versions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.task_versions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: task_versions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.task_versions_id_seq OWNED BY public.task_versions.id;


--
-- Name: tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tasks (
    id integer NOT NULL,
    project_id integer NOT NULL,
    name text NOT NULL,
    description text,
    trigger_type text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    required_capability_code text,
    target_selector_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    schedule text,
    event_rule_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    script text NOT NULL,
    created_by_user_id integer,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    team_id integer NOT NULL,
    current_published_version_id bigint
);


--
-- Name: tasks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.tasks_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tasks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.tasks_id_seq OWNED BY public.tasks.id;


--
-- Name: team_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.team_members (
    id integer NOT NULL,
    team_id integer NOT NULL,
    user_id integer NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: team_members_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.team_members_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: team_members_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.team_members_id_seq OWNED BY public.team_members.id;


--
-- Name: teams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.teams (
    id integer NOT NULL,
    name text NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: teams_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.teams_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: teams_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.teams_id_seq OWNED BY public.teams.id;


--
-- Name: telemetry_event_dedup; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_event_dedup (
    adapter_id bigint NOT NULL,
    event_id text NOT NULL,
    project_id integer NOT NULL,
    captured_at timestamp with time zone NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id integer NOT NULL,
    name text NOT NULL,
    email text,
    phone text,
    password text,
    role text DEFAULT 'user'::text NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: device_telemetry_default; Type: TABLE ATTACH; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_telemetry ATTACH PARTITION public.device_telemetry_default DEFAULT;


--
-- Name: agent_draft_evidence id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_draft_evidence ALTER COLUMN id SET DEFAULT nextval('public.agent_draft_evidence_id_seq'::regclass);


--
-- Name: agent_messages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_messages ALTER COLUMN id SET DEFAULT nextval('public.agent_messages_id_seq'::regclass);


--
-- Name: agent_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_sessions ALTER COLUMN id SET DEFAULT nextval('public.agent_sessions_id_seq'::regclass);


--
-- Name: agents id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents ALTER COLUMN id SET DEFAULT nextval('public.agents_id_seq'::regclass);


--
-- Name: ai_providers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_providers ALTER COLUMN id SET DEFAULT nextval('public.ai_providers_id_seq'::regclass);


--
-- Name: alert_automation_policies id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policies ALTER COLUMN id SET DEFAULT nextval('public.alert_automation_policies_id_seq'::regclass);


--
-- Name: alert_automation_policy_versions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policy_versions ALTER COLUMN id SET DEFAULT nextval('public.alert_automation_policy_versions_id_seq'::regclass);


--
-- Name: algorithm_callback_receipts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_callback_receipts ALTER COLUMN id SET DEFAULT nextval('public.algorithm_callback_receipts_id_seq'::regclass);


--
-- Name: algorithm_definition_versions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definition_versions ALTER COLUMN id SET DEFAULT nextval('public.algorithm_definition_versions_id_seq'::regclass);


--
-- Name: algorithm_definitions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definitions ALTER COLUMN id SET DEFAULT nextval('public.algorithm_definitions_id_seq'::regclass);


--
-- Name: algorithm_providers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_providers ALTER COLUMN id SET DEFAULT nextval('public.algorithm_providers_id_seq'::regclass);


--
-- Name: algorithm_run_attempts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_run_attempts ALTER COLUMN id SET DEFAULT nextval('public.algorithm_run_attempts_id_seq'::regclass);


--
-- Name: approvals id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approvals ALTER COLUMN id SET DEFAULT nextval('public.approvals_id_seq'::regclass);


--
-- Name: asset_derivatives id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_derivatives ALTER COLUMN id SET DEFAULT nextval('public.asset_derivatives_id_seq'::regclass);


--
-- Name: assets id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets ALTER COLUMN id SET DEFAULT nextval('public.assets_id_seq'::regclass);


--
-- Name: audit_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events ALTER COLUMN id SET DEFAULT nextval('public.audit_events_id_seq'::regclass);


--
-- Name: command_attempts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.command_attempts ALTER COLUMN id SET DEFAULT nextval('public.command_attempts_id_seq'::regclass);


--
-- Name: connector_capability_snapshots id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_capability_snapshots ALTER COLUMN id SET DEFAULT nextval('public.connector_capability_snapshots_id_seq'::regclass);


--
-- Name: connector_definitions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_definitions ALTER COLUMN id SET DEFAULT nextval('public.connector_definitions_id_seq'::regclass);


--
-- Name: connector_remote_resources id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_remote_resources ALTER COLUMN id SET DEFAULT nextval('public.connector_remote_resources_id_seq'::regclass);


--
-- Name: connector_resource_sync_states id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_resource_sync_states ALTER COLUMN id SET DEFAULT nextval('public.connector_resource_sync_states_id_seq'::regclass);


--
-- Name: connector_sync_runs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_sync_runs ALTER COLUMN id SET DEFAULT nextval('public.connector_sync_runs_id_seq'::regclass);


--
-- Name: coordinate_references id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coordinate_references ALTER COLUMN id SET DEFAULT nextval('public.coordinate_references_id_seq'::regclass);


--
-- Name: detection_groups id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detection_groups ALTER COLUMN id SET DEFAULT nextval('public.detection_groups_id_seq'::regclass);


--
-- Name: detections id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detections ALTER COLUMN id SET DEFAULT nextval('public.detections_id_seq'::regclass);


--
-- Name: device_adapters id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_adapters ALTER COLUMN id SET DEFAULT nextval('public.device_adapters_id_seq'::regclass);


--
-- Name: device_capabilities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capabilities ALTER COLUMN id SET DEFAULT nextval('public.device_capabilities_id_seq'::regclass);


--
-- Name: device_capability_grants id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capability_grants ALTER COLUMN id SET DEFAULT nextval('public.device_capability_grants_id_seq'::regclass);


--
-- Name: device_command_protocol_correlations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_command_protocol_correlations ALTER COLUMN id SET DEFAULT nextval('public.device_command_protocol_correlations_id_seq'::regclass);


--
-- Name: device_connections id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connections ALTER COLUMN id SET DEFAULT nextval('public.device_connections_id_seq'::regclass);


--
-- Name: device_connector_bindings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connector_bindings ALTER COLUMN id SET DEFAULT nextval('public.device_connector_bindings_id_seq'::regclass);


--
-- Name: device_external_identities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_external_identities ALTER COLUMN id SET DEFAULT nextval('public.device_external_identities_id_seq'::regclass);


--
-- Name: device_network_profiles id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_network_profiles ALTER COLUMN id SET DEFAULT nextval('public.device_network_profiles_id_seq'::regclass);


--
-- Name: device_protocol_messages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_protocol_messages ALTER COLUMN id SET DEFAULT nextval('public.device_protocol_messages_id_seq'::regclass);


--
-- Name: device_relationships id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_relationships ALTER COLUMN id SET DEFAULT nextval('public.device_relationships_id_seq'::regclass);


--
-- Name: device_stream_channels id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_stream_channels ALTER COLUMN id SET DEFAULT nextval('public.device_stream_channels_id_seq'::regclass);


--
-- Name: device_telemetry id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_telemetry ALTER COLUMN id SET DEFAULT nextval('public.device_telemetry_id_seq'::regclass);


--
-- Name: device_types id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_types ALTER COLUMN id SET DEFAULT nextval('public.device_types_id_seq'::regclass);


--
-- Name: devices id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices ALTER COLUMN id SET DEFAULT nextval('public.devices_id_seq'::regclass);


--
-- Name: driver_definitions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.driver_definitions ALTER COLUMN id SET DEFAULT nextval('public.driver_definitions_id_seq'::regclass);


--
-- Name: event_feedback id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_feedback ALTER COLUMN id SET DEFAULT nextval('public.event_feedback_id_seq'::regclass);


--
-- Name: event_rule_versions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rule_versions ALTER COLUMN id SET DEFAULT nextval('public.event_rule_versions_id_seq'::regclass);


--
-- Name: event_rules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rules ALTER COLUMN id SET DEFAULT nextval('public.event_rules_id_seq'::regclass);


--
-- Name: evidence_links id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evidence_links ALTER COLUMN id SET DEFAULT nextval('public.evidence_links_id_seq'::regclass);


--
-- Name: generated_report_evidence id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_report_evidence ALTER COLUMN id SET DEFAULT nextval('public.generated_report_evidence_id_seq'::regclass);


--
-- Name: idempotency_records id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.idempotency_records ALTER COLUMN id SET DEFAULT nextval('public.idempotency_records_id_seq'::regclass);


--
-- Name: issue_assignees id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_assignees ALTER COLUMN id SET DEFAULT nextval('public.issue_assignees_id_seq'::regclass);


--
-- Name: issue_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_events ALTER COLUMN id SET DEFAULT nextval('public.issue_events_id_seq'::regclass);


--
-- Name: issue_feedback id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback ALTER COLUMN id SET DEFAULT nextval('public.issue_feedback_id_seq'::regclass);


--
-- Name: issue_links id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_links ALTER COLUMN id SET DEFAULT nextval('public.issue_links_id_seq'::regclass);


--
-- Name: issues id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issues ALTER COLUMN id SET DEFAULT nextval('public.issues_id_seq'::regclass);


--
-- Name: live_streams id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.live_streams ALTER COLUMN id SET DEFAULT nextval('public.live_streams_id_seq'::regclass);


--
-- Name: observations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.observations ALTER COLUMN id SET DEFAULT nextval('public.observations_id_seq'::regclass);


--
-- Name: outbox_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_events ALTER COLUMN id SET DEFAULT nextval('public.outbox_events_id_seq'::regclass);


--
-- Name: platform_audit_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_audit_events ALTER COLUMN id SET DEFAULT nextval('public.platform_audit_events_id_seq'::regclass);


--
-- Name: project_events cursor; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_events ALTER COLUMN cursor SET DEFAULT nextval('public.project_events_cursor_seq'::regclass);


--
-- Name: project_permissions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_permissions ALTER COLUMN id SET DEFAULT nextval('public.project_permissions_id_seq'::regclass);


--
-- Name: projects id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects ALTER COLUMN id SET DEFAULT nextval('public.projects_id_seq'::regclass);


--
-- Name: retention_policies id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_policies ALTER COLUMN id SET DEFAULT nextval('public.retention_policies_id_seq'::regclass);


--
-- Name: safety_policy_versions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.safety_policy_versions ALTER COLUMN id SET DEFAULT nextval('public.safety_policy_versions_id_seq'::regclass);


--
-- Name: sensor_calibrations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sensor_calibrations ALTER COLUMN id SET DEFAULT nextval('public.sensor_calibrations_id_seq'::regclass);


--
-- Name: task_run_steps id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_run_steps ALTER COLUMN id SET DEFAULT nextval('public.task_run_steps_id_seq'::regclass);


--
-- Name: task_runs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs ALTER COLUMN id SET DEFAULT nextval('public.task_runs_id_seq'::regclass);


--
-- Name: task_steps id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_steps ALTER COLUMN id SET DEFAULT nextval('public.task_steps_id_seq'::regclass);


--
-- Name: task_versions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_versions ALTER COLUMN id SET DEFAULT nextval('public.task_versions_id_seq'::regclass);


--
-- Name: tasks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks ALTER COLUMN id SET DEFAULT nextval('public.tasks_id_seq'::regclass);


--
-- Name: team_members id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_members ALTER COLUMN id SET DEFAULT nextval('public.team_members_id_seq'::regclass);


--
-- Name: teams id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams ALTER COLUMN id SET DEFAULT nextval('public.teams_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: agent_draft_evidence agent_draft_evidence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_draft_evidence
    ADD CONSTRAINT agent_draft_evidence_pkey PRIMARY KEY (id);


--
-- Name: agent_draft_evidence agent_draft_evidence_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_draft_evidence
    ADD CONSTRAINT agent_draft_evidence_unique UNIQUE (agent_draft_id, reference_type, reference_id, reference_version);


--
-- Name: agent_drafts agent_drafts_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_drafts
    ADD CONSTRAINT agent_drafts_id_project_unique UNIQUE (id, project_id);


--
-- Name: agent_drafts agent_drafts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_drafts
    ADD CONSTRAINT agent_drafts_pkey PRIMARY KEY (id);


--
-- Name: agent_messages agent_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_messages
    ADD CONSTRAINT agent_messages_pkey PRIMARY KEY (id);


--
-- Name: agent_sessions agent_sessions_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_sessions
    ADD CONSTRAINT agent_sessions_id_project_unique UNIQUE (id, project_id);


--
-- Name: agent_sessions agent_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_sessions
    ADD CONSTRAINT agent_sessions_pkey PRIMARY KEY (id);


--
-- Name: agent_tool_jobs agent_tool_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_tool_jobs
    ADD CONSTRAINT agent_tool_jobs_pkey PRIMARY KEY (id);


--
-- Name: agents agents_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_id_project_unique UNIQUE (id, project_id);


--
-- Name: agents agents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);


--
-- Name: ai_providers ai_providers_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_providers
    ADD CONSTRAINT ai_providers_name_unique UNIQUE (name);


--
-- Name: ai_providers ai_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_providers
    ADD CONSTRAINT ai_providers_pkey PRIMARY KEY (id);


--
-- Name: alert_automation_drafts alert_automation_drafts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_drafts
    ADD CONSTRAINT alert_automation_drafts_pkey PRIMARY KEY (id);


--
-- Name: alert_automation_drafts alert_automation_drafts_run_type_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_drafts
    ADD CONSTRAINT alert_automation_drafts_run_type_unique UNIQUE (automation_run_id, draft_type);


--
-- Name: alert_automation_policies alert_automation_policies_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policies
    ADD CONSTRAINT alert_automation_policies_id_project_unique UNIQUE (id, project_id);


--
-- Name: alert_automation_policies alert_automation_policies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policies
    ADD CONSTRAINT alert_automation_policies_pkey PRIMARY KEY (id);


--
-- Name: alert_automation_policies alert_automation_policies_project_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policies
    ADD CONSTRAINT alert_automation_policies_project_name_unique UNIQUE (project_id, name);


--
-- Name: alert_automation_policy_versions alert_automation_policy_versions_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policy_versions
    ADD CONSTRAINT alert_automation_policy_versions_id_project_unique UNIQUE (id, project_id);


--
-- Name: alert_automation_policy_versions alert_automation_policy_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policy_versions
    ADD CONSTRAINT alert_automation_policy_versions_pkey PRIMARY KEY (id);


--
-- Name: alert_automation_policy_versions alert_automation_policy_versions_policy_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policy_versions
    ADD CONSTRAINT alert_automation_policy_versions_policy_version_unique UNIQUE (alert_automation_policy_id, version);


--
-- Name: alert_automation_runs alert_automation_runs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_runs
    ADD CONSTRAINT alert_automation_runs_id_project_unique UNIQUE (id, project_id);


--
-- Name: alert_automation_runs alert_automation_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_runs
    ADD CONSTRAINT alert_automation_runs_pkey PRIMARY KEY (id);


--
-- Name: algorithm_callback_receipts algorithm_callback_receipts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_callback_receipts
    ADD CONSTRAINT algorithm_callback_receipts_pkey PRIMARY KEY (id);


--
-- Name: algorithm_callback_receipts algorithm_callback_receipts_provider_callback_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_callback_receipts
    ADD CONSTRAINT algorithm_callback_receipts_provider_callback_unique UNIQUE (provider_id, callback_id);


--
-- Name: algorithm_definition_versions algorithm_definition_versions_definition_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definition_versions
    ADD CONSTRAINT algorithm_definition_versions_definition_version_unique UNIQUE (algorithm_definition_id, version);


--
-- Name: algorithm_definition_versions algorithm_definition_versions_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definition_versions
    ADD CONSTRAINT algorithm_definition_versions_id_project_unique UNIQUE (id, project_id);


--
-- Name: algorithm_definition_versions algorithm_definition_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definition_versions
    ADD CONSTRAINT algorithm_definition_versions_pkey PRIMARY KEY (id);


--
-- Name: algorithm_definitions algorithm_definitions_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definitions
    ADD CONSTRAINT algorithm_definitions_id_project_unique UNIQUE (id, project_id);


--
-- Name: algorithm_definitions algorithm_definitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definitions
    ADD CONSTRAINT algorithm_definitions_pkey PRIMARY KEY (id);


--
-- Name: algorithm_definitions algorithm_definitions_project_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definitions
    ADD CONSTRAINT algorithm_definitions_project_name_unique UNIQUE (project_id, name);


--
-- Name: algorithm_providers algorithm_providers_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_providers
    ADD CONSTRAINT algorithm_providers_id_project_unique UNIQUE (id, project_id);


--
-- Name: algorithm_providers algorithm_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_providers
    ADD CONSTRAINT algorithm_providers_pkey PRIMARY KEY (id);


--
-- Name: algorithm_providers algorithm_providers_project_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_providers
    ADD CONSTRAINT algorithm_providers_project_name_unique UNIQUE (project_id, name);


--
-- Name: algorithm_run_attempts algorithm_run_attempts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_run_attempts
    ADD CONSTRAINT algorithm_run_attempts_pkey PRIMARY KEY (id);


--
-- Name: algorithm_run_attempts algorithm_run_attempts_run_attempt_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_run_attempts
    ADD CONSTRAINT algorithm_run_attempts_run_attempt_unique UNIQUE (algorithm_run_id, attempt);


--
-- Name: algorithm_runs algorithm_runs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_runs
    ADD CONSTRAINT algorithm_runs_id_project_unique UNIQUE (id, project_id);


--
-- Name: algorithm_runs algorithm_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_runs
    ADD CONSTRAINT algorithm_runs_pkey PRIMARY KEY (id);


--
-- Name: algorithm_runs algorithm_runs_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_runs
    ADD CONSTRAINT algorithm_runs_project_idempotency_unique UNIQUE (project_id, idempotency_key);


--
-- Name: approval_requests approval_requests_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_id_project_unique UNIQUE (id, project_id);


--
-- Name: approval_requests approval_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_pkey PRIMARY KEY (id);


--
-- Name: approvals approvals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approvals
    ADD CONSTRAINT approvals_pkey PRIMARY KEY (id);


--
-- Name: approvals approvals_request_approver_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approvals
    ADD CONSTRAINT approvals_request_approver_unique UNIQUE (approval_request_id, approver_user_id);


--
-- Name: asset_derivatives asset_derivatives_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_derivatives
    ADD CONSTRAINT asset_derivatives_pkey PRIMARY KEY (id);


--
-- Name: asset_derivatives asset_derivatives_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_derivatives
    ADD CONSTRAINT asset_derivatives_unique UNIQUE (source_asset_id, derived_asset_id, derivative_type);


--
-- Name: asset_upload_intents asset_upload_intents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_upload_intents
    ADD CONSTRAINT asset_upload_intents_pkey PRIMARY KEY (id);


--
-- Name: asset_upload_intents asset_upload_intents_project_object_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_upload_intents
    ADD CONSTRAINT asset_upload_intents_project_object_unique UNIQUE (project_id, object_key);


--
-- Name: assets assets_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_id_project_unique UNIQUE (id, project_id);


--
-- Name: assets assets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_pkey PRIMARY KEY (id);


--
-- Name: assets assets_project_logical_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_project_logical_version_unique UNIQUE (project_id, logical_key, version);


--
-- Name: audit_events audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id);


--
-- Name: command_attempts command_attempts_command_attempt_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.command_attempts
    ADD CONSTRAINT command_attempts_command_attempt_unique UNIQUE (command_id, attempt);


--
-- Name: command_attempts command_attempts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.command_attempts
    ADD CONSTRAINT command_attempts_pkey PRIMARY KEY (id);


--
-- Name: connector_action_jobs connector_action_jobs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_action_jobs connector_action_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_pkey PRIMARY KEY (id);


--
-- Name: connector_action_jobs connector_action_jobs_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_project_idempotency_unique UNIQUE (project_id, connector_instance_id, action_kind, idempotency_key);


--
-- Name: connector_asset_access_refs connector_asset_access_refs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_asset_access_refs
    ADD CONSTRAINT connector_asset_access_refs_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_asset_access_refs connector_asset_access_refs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_asset_access_refs
    ADD CONSTRAINT connector_asset_access_refs_pkey PRIMARY KEY (id);


--
-- Name: connector_asset_access_refs connector_asset_access_refs_reference_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_asset_access_refs
    ADD CONSTRAINT connector_asset_access_refs_reference_unique UNIQUE (project_id, connector_instance_id, access_kind, reference_digest);


--
-- Name: connector_capability_snapshots connector_capability_snapshots_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_capability_snapshots
    ADD CONSTRAINT connector_capability_snapshots_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_capability_snapshots connector_capability_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_capability_snapshots
    ADD CONSTRAINT connector_capability_snapshots_pkey PRIMARY KEY (id);


--
-- Name: connector_control_sessions connector_control_sessions_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_sessions
    ADD CONSTRAINT connector_control_sessions_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_control_sessions connector_control_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_sessions
    ADD CONSTRAINT connector_control_sessions_pkey PRIMARY KEY (id);


--
-- Name: connector_control_sessions connector_control_sessions_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_sessions
    ADD CONSTRAINT connector_control_sessions_project_idempotency_unique UNIQUE (project_id, device_id, idempotency_key);


--
-- Name: connector_definitions connector_definitions_key_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_definitions
    ADD CONSTRAINT connector_definitions_key_version_unique UNIQUE (connector_key, version);


--
-- Name: connector_definitions connector_definitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_definitions
    ADD CONSTRAINT connector_definitions_pkey PRIMARY KEY (id);


--
-- Name: connector_device_admin_jobs connector_device_admin_jobs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_device_admin_jobs
    ADD CONSTRAINT connector_device_admin_jobs_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_device_admin_jobs connector_device_admin_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_device_admin_jobs
    ADD CONSTRAINT connector_device_admin_jobs_pkey PRIMARY KEY (id);


--
-- Name: connector_device_admin_jobs connector_device_admin_jobs_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_device_admin_jobs
    ADD CONSTRAINT connector_device_admin_jobs_project_idempotency_unique UNIQUE (project_id, connector_instance_id, action_kind, idempotency_key);


--
-- Name: connector_geospatial_action_jobs connector_geospatial_action_jobs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_geospatial_action_jobs
    ADD CONSTRAINT connector_geospatial_action_jobs_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_geospatial_action_jobs connector_geospatial_action_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_geospatial_action_jobs
    ADD CONSTRAINT connector_geospatial_action_jobs_pkey PRIMARY KEY (id);


--
-- Name: connector_geospatial_action_jobs connector_geospatial_action_jobs_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_geospatial_action_jobs
    ADD CONSTRAINT connector_geospatial_action_jobs_project_idempotency_unique UNIQUE (project_id, connector_instance_id, action_kind, idempotency_key);


--
-- Name: connector_live_action_jobs connector_live_action_jobs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_live_action_jobs
    ADD CONSTRAINT connector_live_action_jobs_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_live_action_jobs connector_live_action_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_live_action_jobs
    ADD CONSTRAINT connector_live_action_jobs_pkey PRIMARY KEY (id);


--
-- Name: connector_live_action_jobs connector_live_action_jobs_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_live_action_jobs
    ADD CONSTRAINT connector_live_action_jobs_project_idempotency_unique UNIQUE (project_id, connector_instance_id, action_kind, idempotency_key);


--
-- Name: connector_management_write_jobs connector_management_write_jobs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_management_write_jobs
    ADD CONSTRAINT connector_management_write_jobs_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_management_write_jobs connector_management_write_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_management_write_jobs
    ADD CONSTRAINT connector_management_write_jobs_pkey PRIMARY KEY (id);


--
-- Name: connector_management_write_jobs connector_management_write_jobs_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_management_write_jobs
    ADD CONSTRAINT connector_management_write_jobs_project_idempotency_unique UNIQUE (project_id, connector_instance_id, action_kind, idempotency_key);


--
-- Name: connector_model_delete_jobs connector_model_delete_jobs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_delete_jobs
    ADD CONSTRAINT connector_model_delete_jobs_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_model_delete_jobs connector_model_delete_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_delete_jobs
    ADD CONSTRAINT connector_model_delete_jobs_pkey PRIMARY KEY (id);


--
-- Name: connector_model_delete_jobs connector_model_delete_jobs_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_delete_jobs
    ADD CONSTRAINT connector_model_delete_jobs_project_idempotency_unique UNIQUE (project_id, connector_instance_id, action_kind, idempotency_key);


--
-- Name: connector_model_jobs connector_model_jobs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_jobs
    ADD CONSTRAINT connector_model_jobs_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_model_jobs connector_model_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_jobs
    ADD CONSTRAINT connector_model_jobs_pkey PRIMARY KEY (id);


--
-- Name: connector_model_jobs connector_model_jobs_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_jobs
    ADD CONSTRAINT connector_model_jobs_project_idempotency_unique UNIQUE (project_id, connector_instance_id, action_kind, idempotency_key);


--
-- Name: connector_object_upload_jobs connector_object_upload_jobs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_object_upload_jobs
    ADD CONSTRAINT connector_object_upload_jobs_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_object_upload_jobs connector_object_upload_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_object_upload_jobs
    ADD CONSTRAINT connector_object_upload_jobs_pkey PRIMARY KEY (id);


--
-- Name: connector_object_upload_jobs connector_object_upload_jobs_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_object_upload_jobs
    ADD CONSTRAINT connector_object_upload_jobs_project_idempotency_unique UNIQUE (project_id, connector_instance_id, operation_kind, idempotency_key);


--
-- Name: connector_open_model_uploads connector_open_model_uploads_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_open_model_uploads
    ADD CONSTRAINT connector_open_model_uploads_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_open_model_uploads connector_open_model_uploads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_open_model_uploads
    ADD CONSTRAINT connector_open_model_uploads_pkey PRIMARY KEY (id);


--
-- Name: connector_open_model_uploads connector_open_model_uploads_project_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_open_model_uploads
    ADD CONSTRAINT connector_open_model_uploads_project_idempotency_unique UNIQUE (project_id, connector_instance_id, idempotency_key);


--
-- Name: connector_remote_resources connector_remote_resources_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_remote_resources
    ADD CONSTRAINT connector_remote_resources_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_remote_resources connector_remote_resources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_remote_resources
    ADD CONSTRAINT connector_remote_resources_pkey PRIMARY KEY (id);


--
-- Name: connector_remote_resources connector_remote_resources_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_remote_resources
    ADD CONSTRAINT connector_remote_resources_unique UNIQUE (project_id, connector_instance_id, resource_kind, remote_id);


--
-- Name: connector_resource_sync_states connector_resource_sync_states_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_resource_sync_states
    ADD CONSTRAINT connector_resource_sync_states_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_resource_sync_states connector_resource_sync_states_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_resource_sync_states
    ADD CONSTRAINT connector_resource_sync_states_pkey PRIMARY KEY (id);


--
-- Name: connector_resource_sync_states connector_resource_sync_states_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_resource_sync_states
    ADD CONSTRAINT connector_resource_sync_states_unique UNIQUE (project_id, connector_instance_id, resource_kind);


--
-- Name: connector_sync_runs connector_sync_runs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_sync_runs
    ADD CONSTRAINT connector_sync_runs_id_project_unique UNIQUE (id, project_id);


--
-- Name: connector_sync_runs connector_sync_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_sync_runs
    ADD CONSTRAINT connector_sync_runs_pkey PRIMARY KEY (id);


--
-- Name: coordinate_references coordinate_references_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coordinate_references
    ADD CONSTRAINT coordinate_references_id_project_unique UNIQUE (id, project_id);


--
-- Name: coordinate_references coordinate_references_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coordinate_references
    ADD CONSTRAINT coordinate_references_pkey PRIMARY KEY (id);


--
-- Name: coordinate_references coordinate_references_project_code_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coordinate_references
    ADD CONSTRAINT coordinate_references_project_code_version_unique UNIQUE (project_id, code, transform_version);


--
-- Name: detection_group_members detection_group_members_detection_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detection_group_members
    ADD CONSTRAINT detection_group_members_detection_unique UNIQUE (detection_id);


--
-- Name: detection_group_members detection_group_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detection_group_members
    ADD CONSTRAINT detection_group_members_pkey PRIMARY KEY (detection_group_id, detection_id);


--
-- Name: detection_groups detection_groups_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detection_groups
    ADD CONSTRAINT detection_groups_id_project_unique UNIQUE (id, project_id);


--
-- Name: detection_groups detection_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detection_groups
    ADD CONSTRAINT detection_groups_pkey PRIMARY KEY (id);


--
-- Name: detections detections_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detections
    ADD CONSTRAINT detections_id_project_unique UNIQUE (id, project_id);


--
-- Name: detections detections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detections
    ADD CONSTRAINT detections_pkey PRIMARY KEY (id);


--
-- Name: detections detections_run_key_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detections
    ADD CONSTRAINT detections_run_key_unique UNIQUE (algorithm_run_id, detection_key);


--
-- Name: device_adapters device_adapters_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_adapters
    ADD CONSTRAINT device_adapters_id_project_unique UNIQUE (id, project_id);


--
-- Name: device_adapters device_adapters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_adapters
    ADD CONSTRAINT device_adapters_pkey PRIMARY KEY (id);


--
-- Name: device_adapters device_adapters_project_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_adapters
    ADD CONSTRAINT device_adapters_project_name_unique UNIQUE (project_id, name);


--
-- Name: device_capabilities device_capabilities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capabilities
    ADD CONSTRAINT device_capabilities_pkey PRIMARY KEY (id);


--
-- Name: device_capability_grants device_capability_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capability_grants
    ADD CONSTRAINT device_capability_grants_pkey PRIMARY KEY (id);


--
-- Name: device_command_protocol_correlations device_command_protocol_correlations_business_method_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_command_protocol_correlations
    ADD CONSTRAINT device_command_protocol_correlations_business_method_unique UNIQUE (adapter_id, business_id, method);


--
-- Name: device_command_protocol_correlations device_command_protocol_correlations_command_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_command_protocol_correlations
    ADD CONSTRAINT device_command_protocol_correlations_command_unique UNIQUE (command_id);


--
-- Name: device_command_protocol_correlations device_command_protocol_correlations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_command_protocol_correlations
    ADD CONSTRAINT device_command_protocol_correlations_pkey PRIMARY KEY (id);


--
-- Name: device_command_protocol_correlations device_command_protocol_correlations_transaction_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_command_protocol_correlations
    ADD CONSTRAINT device_command_protocol_correlations_transaction_unique UNIQUE (adapter_id, transaction_id);


--
-- Name: device_commands device_commands_device_idempotency_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_commands
    ADD CONSTRAINT device_commands_device_idempotency_unique UNIQUE (device_id, idempotency_key);


--
-- Name: device_commands device_commands_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_commands
    ADD CONSTRAINT device_commands_id_project_unique UNIQUE (id, project_id);


--
-- Name: device_commands device_commands_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_commands
    ADD CONSTRAINT device_commands_pkey PRIMARY KEY (id);


--
-- Name: device_connections device_connections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connections
    ADD CONSTRAINT device_connections_pkey PRIMARY KEY (id);


--
-- Name: device_connections device_connections_session_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connections
    ADD CONSTRAINT device_connections_session_unique UNIQUE (adapter_id, session_key);


--
-- Name: device_connector_bindings device_connector_bindings_device_connector_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connector_bindings
    ADD CONSTRAINT device_connector_bindings_device_connector_unique UNIQUE (device_id, connector_instance_id);


--
-- Name: device_connector_bindings device_connector_bindings_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connector_bindings
    ADD CONSTRAINT device_connector_bindings_id_project_unique UNIQUE (id, project_id);


--
-- Name: device_connector_bindings device_connector_bindings_identity_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connector_bindings
    ADD CONSTRAINT device_connector_bindings_identity_unique UNIQUE (external_identity_id);


--
-- Name: device_connector_bindings device_connector_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connector_bindings
    ADD CONSTRAINT device_connector_bindings_pkey PRIMARY KEY (id);


--
-- Name: device_external_identities device_external_identities_adapter_external_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_external_identities
    ADD CONSTRAINT device_external_identities_adapter_external_unique UNIQUE (adapter_id, external_device_id);


--
-- Name: device_external_identities device_external_identities_connector_identity_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_external_identities
    ADD CONSTRAINT device_external_identities_connector_identity_project_unique UNIQUE (id, adapter_id, project_id);


--
-- Name: device_external_identities device_external_identities_device_adapter_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_external_identities
    ADD CONSTRAINT device_external_identities_device_adapter_unique UNIQUE (device_id, adapter_id);


--
-- Name: device_external_identities device_external_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_external_identities
    ADD CONSTRAINT device_external_identities_pkey PRIMARY KEY (id);


--
-- Name: device_latest_telemetry device_latest_telemetry_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_latest_telemetry
    ADD CONSTRAINT device_latest_telemetry_pkey PRIMARY KEY (device_id);


--
-- Name: device_network_profiles device_network_profiles_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_network_profiles
    ADD CONSTRAINT device_network_profiles_id_project_unique UNIQUE (id, project_id);


--
-- Name: device_network_profiles device_network_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_network_profiles
    ADD CONSTRAINT device_network_profiles_pkey PRIMARY KEY (id);


--
-- Name: device_network_profiles device_network_profiles_project_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_network_profiles
    ADD CONSTRAINT device_network_profiles_project_name_unique UNIQUE (project_id, name);


--
-- Name: device_protocol_cursors device_protocol_cursors_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_protocol_cursors
    ADD CONSTRAINT device_protocol_cursors_pkey PRIMARY KEY (adapter_id, route_key);


--
-- Name: device_protocol_messages device_protocol_messages_adapter_topic_tid_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_protocol_messages
    ADD CONSTRAINT device_protocol_messages_adapter_topic_tid_unique UNIQUE (adapter_id, topic, transaction_id);


--
-- Name: device_protocol_messages device_protocol_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_protocol_messages
    ADD CONSTRAINT device_protocol_messages_pkey PRIMARY KEY (id);


--
-- Name: device_relationships device_relationships_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_relationships
    ADD CONSTRAINT device_relationships_id_project_unique UNIQUE (id, project_id);


--
-- Name: device_relationships device_relationships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_relationships
    ADD CONSTRAINT device_relationships_pkey PRIMARY KEY (id);


--
-- Name: device_relationships device_relationships_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_relationships
    ADD CONSTRAINT device_relationships_unique UNIQUE (project_id, from_device_id, to_device_id, relation_type, valid_from);


--
-- Name: device_stream_channels device_stream_channels_device_key_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_stream_channels
    ADD CONSTRAINT device_stream_channels_device_key_unique UNIQUE (device_id, channel_key);


--
-- Name: device_stream_channels device_stream_channels_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_stream_channels
    ADD CONSTRAINT device_stream_channels_id_project_unique UNIQUE (id, project_id);


--
-- Name: device_stream_channels device_stream_channels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_stream_channels
    ADD CONSTRAINT device_stream_channels_pkey PRIMARY KEY (id);


--
-- Name: device_stream_channels device_stream_channels_project_stable_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_stream_channels
    ADD CONSTRAINT device_stream_channels_project_stable_unique UNIQUE (project_id, stable_channel_id);


--
-- Name: device_telemetry device_telemetry_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_telemetry
    ADD CONSTRAINT device_telemetry_pkey PRIMARY KEY (id, captured_at);


--
-- Name: device_telemetry_default device_telemetry_default_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_telemetry_default
    ADD CONSTRAINT device_telemetry_default_pkey PRIMARY KEY (id, captured_at);


--
-- Name: device_types device_types_key_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_types
    ADD CONSTRAINT device_types_key_version_unique UNIQUE (type_key, version);


--
-- Name: device_types device_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_types
    ADD CONSTRAINT device_types_pkey PRIMARY KEY (id);


--
-- Name: devices devices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_pkey PRIMARY KEY (id);


--
-- Name: driver_definitions driver_definitions_key_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.driver_definitions
    ADD CONSTRAINT driver_definitions_key_version_unique UNIQUE (driver_key, version);


--
-- Name: driver_definitions driver_definitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.driver_definitions
    ADD CONSTRAINT driver_definitions_pkey PRIMARY KEY (id);


--
-- Name: event_feedback event_feedback_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_feedback
    ADD CONSTRAINT event_feedback_pkey PRIMARY KEY (id);


--
-- Name: event_rule_versions event_rule_versions_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rule_versions
    ADD CONSTRAINT event_rule_versions_id_project_unique UNIQUE (id, project_id);


--
-- Name: event_rule_versions event_rule_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rule_versions
    ADD CONSTRAINT event_rule_versions_pkey PRIMARY KEY (id);


--
-- Name: event_rule_versions event_rule_versions_rule_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rule_versions
    ADD CONSTRAINT event_rule_versions_rule_version_unique UNIQUE (event_rule_id, version);


--
-- Name: event_rules event_rules_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rules
    ADD CONSTRAINT event_rules_id_project_unique UNIQUE (id, project_id);


--
-- Name: event_rules event_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rules
    ADD CONSTRAINT event_rules_pkey PRIMARY KEY (id);


--
-- Name: event_rules event_rules_project_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rules
    ADD CONSTRAINT event_rules_project_name_unique UNIQUE (project_id, name);


--
-- Name: evidence_links evidence_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evidence_links
    ADD CONSTRAINT evidence_links_pkey PRIMARY KEY (id);


--
-- Name: evidence_links evidence_links_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evidence_links
    ADD CONSTRAINT evidence_links_unique UNIQUE (project_id, target_type, target_id, asset_id, start_offset_ms, end_offset_ms);


--
-- Name: generated_report_evidence generated_report_evidence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_report_evidence
    ADD CONSTRAINT generated_report_evidence_pkey PRIMARY KEY (id);


--
-- Name: generated_report_evidence generated_report_evidence_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_report_evidence
    ADD CONSTRAINT generated_report_evidence_version_unique UNIQUE (report_version_id, evidence_type, evidence_id, evidence_version);


--
-- Name: generated_report_versions generated_report_versions_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_report_versions
    ADD CONSTRAINT generated_report_versions_id_project_unique UNIQUE (id, project_id);


--
-- Name: generated_report_versions generated_report_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_report_versions
    ADD CONSTRAINT generated_report_versions_pkey PRIMARY KEY (id);


--
-- Name: generated_report_versions generated_report_versions_report_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_report_versions
    ADD CONSTRAINT generated_report_versions_report_version_unique UNIQUE (generated_report_id, version);


--
-- Name: generated_reports generated_reports_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_reports
    ADD CONSTRAINT generated_reports_id_project_unique UNIQUE (id, project_id);


--
-- Name: generated_reports generated_reports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_reports
    ADD CONSTRAINT generated_reports_pkey PRIMARY KEY (id);


--
-- Name: generated_reports generated_reports_project_source_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_reports
    ADD CONSTRAINT generated_reports_project_source_unique UNIQUE (project_id, source_type, source_id);


--
-- Name: idempotency_records idempotency_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.idempotency_records
    ADD CONSTRAINT idempotency_records_pkey PRIMARY KEY (id);


--
-- Name: idempotency_records idempotency_records_scope_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.idempotency_records
    ADD CONSTRAINT idempotency_records_scope_unique UNIQUE (project_id, actor_key, operation, idempotency_key);


--
-- Name: issue_assignees issue_assignees_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_assignees
    ADD CONSTRAINT issue_assignees_pkey PRIMARY KEY (id);


--
-- Name: issue_events issue_events_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_events
    ADD CONSTRAINT issue_events_id_project_unique UNIQUE (id, project_id);


--
-- Name: issue_events issue_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_events
    ADD CONSTRAINT issue_events_pkey PRIMARY KEY (id);


--
-- Name: issue_feedback issue_feedback_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback
    ADD CONSTRAINT issue_feedback_id_project_unique UNIQUE (id, project_id);


--
-- Name: issue_feedback issue_feedback_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback
    ADD CONSTRAINT issue_feedback_pkey PRIMARY KEY (id);


--
-- Name: issue_feedback issue_feedback_project_client_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback
    ADD CONSTRAINT issue_feedback_project_client_unique UNIQUE (project_id, client_key);


--
-- Name: issue_links issue_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_links
    ADD CONSTRAINT issue_links_pkey PRIMARY KEY (id);


--
-- Name: issues issues_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issues
    ADD CONSTRAINT issues_id_project_unique UNIQUE (id, project_id);


--
-- Name: issues issues_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issues
    ADD CONSTRAINT issues_pkey PRIMARY KEY (id);


--
-- Name: live_streams live_streams_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.live_streams
    ADD CONSTRAINT live_streams_id_project_unique UNIQUE (id, project_id);


--
-- Name: live_streams live_streams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.live_streams
    ADD CONSTRAINT live_streams_pkey PRIMARY KEY (id);


--
-- Name: observations observations_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.observations
    ADD CONSTRAINT observations_id_project_unique UNIQUE (id, project_id);


--
-- Name: observations observations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.observations
    ADD CONSTRAINT observations_pkey PRIMARY KEY (id);


--
-- Name: observations observations_source_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.observations
    ADD CONSTRAINT observations_source_unique UNIQUE (adapter_id, source_event_id);


--
-- Name: outbox_consumptions outbox_consumptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_consumptions
    ADD CONSTRAINT outbox_consumptions_pkey PRIMARY KEY (consumer_name, event_id);


--
-- Name: outbox_events outbox_events_event_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_events
    ADD CONSTRAINT outbox_events_event_unique UNIQUE (event_id);


--
-- Name: outbox_events outbox_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_events
    ADD CONSTRAINT outbox_events_pkey PRIMARY KEY (id);


--
-- Name: perception_events perception_events_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.perception_events
    ADD CONSTRAINT perception_events_id_project_unique UNIQUE (id, project_id);


--
-- Name: perception_events perception_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.perception_events
    ADD CONSTRAINT perception_events_pkey PRIMARY KEY (id);


--
-- Name: platform_audit_events platform_audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_audit_events
    ADD CONSTRAINT platform_audit_events_pkey PRIMARY KEY (id);


--
-- Name: poses poses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.poses
    ADD CONSTRAINT poses_pkey PRIMARY KEY (observation_id);


--
-- Name: project_events project_events_event_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_events
    ADD CONSTRAINT project_events_event_unique UNIQUE (event_id);


--
-- Name: project_events project_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_events
    ADD CONSTRAINT project_events_pkey PRIMARY KEY (cursor);


--
-- Name: project_feature_flags project_feature_flags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_feature_flags
    ADD CONSTRAINT project_feature_flags_pkey PRIMARY KEY (project_id);


--
-- Name: project_permissions project_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_permissions
    ADD CONSTRAINT project_permissions_pkey PRIMARY KEY (id);


--
-- Name: project_permissions project_permissions_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_permissions
    ADD CONSTRAINT project_permissions_unique UNIQUE (project_id, user_id, permission);


--
-- Name: projects projects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);


--
-- Name: retention_cleanup_runs retention_cleanup_runs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_cleanup_runs
    ADD CONSTRAINT retention_cleanup_runs_id_project_unique UNIQUE (id, project_id);


--
-- Name: retention_cleanup_runs retention_cleanup_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_cleanup_runs
    ADD CONSTRAINT retention_cleanup_runs_pkey PRIMARY KEY (id);


--
-- Name: retention_deletion_tombstones retention_deletion_tombstones_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_deletion_tombstones
    ADD CONSTRAINT retention_deletion_tombstones_pkey PRIMARY KEY (id);


--
-- Name: retention_holds retention_holds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_holds
    ADD CONSTRAINT retention_holds_pkey PRIMARY KEY (id);


--
-- Name: retention_policies retention_policies_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_policies
    ADD CONSTRAINT retention_policies_id_project_unique UNIQUE (id, project_id);


--
-- Name: retention_policies retention_policies_key_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_policies
    ADD CONSTRAINT retention_policies_key_version_unique UNIQUE (project_id, policy_key, version);


--
-- Name: retention_policies retention_policies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_policies
    ADD CONSTRAINT retention_policies_pkey PRIMARY KEY (id);


--
-- Name: retention_deletion_tombstones retention_tombstones_asset_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_deletion_tombstones
    ADD CONSTRAINT retention_tombstones_asset_unique UNIQUE (asset_id);


--
-- Name: safety_policy_versions safety_policy_versions_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.safety_policy_versions
    ADD CONSTRAINT safety_policy_versions_id_project_unique UNIQUE (id, project_id);


--
-- Name: safety_policy_versions safety_policy_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.safety_policy_versions
    ADD CONSTRAINT safety_policy_versions_pkey PRIMARY KEY (id);


--
-- Name: safety_policy_versions safety_policy_versions_project_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.safety_policy_versions
    ADD CONSTRAINT safety_policy_versions_project_version_unique UNIQUE (project_id, version);


--
-- Name: sensor_calibrations sensor_calibrations_device_sensor_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sensor_calibrations
    ADD CONSTRAINT sensor_calibrations_device_sensor_version_unique UNIQUE (device_id, sensor_key, version);


--
-- Name: sensor_calibrations sensor_calibrations_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sensor_calibrations
    ADD CONSTRAINT sensor_calibrations_id_project_unique UNIQUE (id, project_id);


--
-- Name: sensor_calibrations sensor_calibrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sensor_calibrations
    ADD CONSTRAINT sensor_calibrations_pkey PRIMARY KEY (id);


--
-- Name: sessions sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (token);


--
-- Name: task_run_steps task_run_steps_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_run_steps
    ADD CONSTRAINT task_run_steps_id_project_unique UNIQUE (id, project_id);


--
-- Name: task_run_steps task_run_steps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_run_steps
    ADD CONSTRAINT task_run_steps_pkey PRIMARY KEY (id);


--
-- Name: task_run_steps task_run_steps_project_execution_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_run_steps
    ADD CONSTRAINT task_run_steps_project_execution_unique UNIQUE (project_id, execution_key);


--
-- Name: task_run_steps task_run_steps_run_position_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_run_steps
    ADD CONSTRAINT task_run_steps_run_position_unique UNIQUE (task_run_id, "position");


--
-- Name: task_runs task_runs_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_id_project_unique UNIQUE (id, project_id);


--
-- Name: task_runs task_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_pkey PRIMARY KEY (id);


--
-- Name: task_steps task_steps_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_steps
    ADD CONSTRAINT task_steps_id_project_unique UNIQUE (id, project_id);


--
-- Name: task_steps task_steps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_steps
    ADD CONSTRAINT task_steps_pkey PRIMARY KEY (id);


--
-- Name: task_steps task_steps_version_key_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_steps
    ADD CONSTRAINT task_steps_version_key_unique UNIQUE (task_version_id, step_key);


--
-- Name: task_steps task_steps_version_position_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_steps
    ADD CONSTRAINT task_steps_version_position_unique UNIQUE (task_version_id, "position");


--
-- Name: task_versions task_versions_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_versions
    ADD CONSTRAINT task_versions_id_project_unique UNIQUE (id, project_id);


--
-- Name: task_versions task_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_versions
    ADD CONSTRAINT task_versions_pkey PRIMARY KEY (id);


--
-- Name: task_versions task_versions_task_version_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_versions
    ADD CONSTRAINT task_versions_task_version_unique UNIQUE (task_id, version);


--
-- Name: tasks tasks_id_project_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_id_project_unique UNIQUE (id, project_id);


--
-- Name: tasks tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);


--
-- Name: team_members team_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT team_members_pkey PRIMARY KEY (id);


--
-- Name: teams teams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_pkey PRIMARY KEY (id);


--
-- Name: telemetry_event_dedup telemetry_event_dedup_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_event_dedup
    ADD CONSTRAINT telemetry_event_dedup_pkey PRIMARY KEY (adapter_id, event_id);


--
-- Name: users users_email_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_unique UNIQUE (email);


--
-- Name: users users_phone_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_phone_unique UNIQUE (phone);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: agent_draft_evidence_project_ref_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_draft_evidence_project_ref_idx ON public.agent_draft_evidence USING btree (project_id, reference_type, reference_id);


--
-- Name: agent_drafts_project_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_drafts_project_created_idx ON public.agent_drafts USING btree (project_id, created_at DESC);


--
-- Name: agent_drafts_session_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_drafts_session_created_idx ON public.agent_drafts USING btree (session_id, created_at DESC);


--
-- Name: agent_messages_session_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_messages_session_created_idx ON public.agent_messages USING btree (session_id, created_at);


--
-- Name: agent_sessions_issue_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_sessions_issue_idx ON public.agent_sessions USING btree (issue_id);


--
-- Name: agent_sessions_project_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_sessions_project_created_idx ON public.agent_sessions USING btree (project_id, created_at);


--
-- Name: agent_sessions_task_run_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_sessions_task_run_idx ON public.agent_sessions USING btree (task_run_id);


--
-- Name: agent_tool_jobs_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_tool_jobs_claim_idx ON public.agent_tool_jobs USING btree (status, created_at) WHERE (status = 'queued'::text);


--
-- Name: agent_tool_jobs_issue_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_tool_jobs_issue_created_idx ON public.agent_tool_jobs USING btree (issue_id, created_at DESC) WHERE (issue_id IS NOT NULL);


--
-- Name: agent_tool_jobs_project_idempotency_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX agent_tool_jobs_project_idempotency_unique ON public.agent_tool_jobs USING btree (project_id, idempotency_key) WHERE (idempotency_key IS NOT NULL);


--
-- Name: agent_tool_jobs_session_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_tool_jobs_session_idx ON public.agent_tool_jobs USING btree (session_id, created_at DESC);


--
-- Name: agents_project_copilot_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX agents_project_copilot_unique ON public.agents USING btree (project_id, ((config_json ->> 'kind'::text))) WHERE ((config_json ->> 'kind'::text) = 'copilot'::text);


--
-- Name: agents_project_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX agents_project_name_unique ON public.agents USING btree (project_id, name);


--
-- Name: agents_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agents_project_status_idx ON public.agents USING btree (project_id, status);


--
-- Name: ai_providers_single_default_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ai_providers_single_default_idx ON public.ai_providers USING btree (is_default) WHERE is_default;


--
-- Name: alert_automation_drafts_project_event_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX alert_automation_drafts_project_event_idx ON public.alert_automation_drafts USING btree (project_id, perception_event_id, created_at DESC);


--
-- Name: alert_automation_policy_versions_one_draft_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX alert_automation_policy_versions_one_draft_idx ON public.alert_automation_policy_versions USING btree (alert_automation_policy_id) WHERE (status = 'draft'::text);


--
-- Name: alert_automation_policy_versions_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX alert_automation_policy_versions_project_status_idx ON public.alert_automation_policy_versions USING btree (project_id, status);


--
-- Name: alert_automation_runs_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX alert_automation_runs_claim_idx ON public.alert_automation_runs USING btree (status, queued_at) WHERE (status = 'queued'::text);


--
-- Name: alert_automation_runs_project_event_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX alert_automation_runs_project_event_idx ON public.alert_automation_runs USING btree (project_id, perception_event_id, created_at DESC);


--
-- Name: algorithm_callback_receipts_run_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX algorithm_callback_receipts_run_idx ON public.algorithm_callback_receipts USING btree (algorithm_run_id, received_at DESC);


--
-- Name: algorithm_definition_versions_one_draft_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX algorithm_definition_versions_one_draft_idx ON public.algorithm_definition_versions USING btree (algorithm_definition_id) WHERE (status = 'draft'::text);


--
-- Name: algorithm_definition_versions_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX algorithm_definition_versions_project_status_idx ON public.algorithm_definition_versions USING btree (project_id, status);


--
-- Name: algorithm_definitions_project_provider_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX algorithm_definitions_project_provider_idx ON public.algorithm_definitions USING btree (project_id, provider_id);


--
-- Name: algorithm_providers_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX algorithm_providers_project_status_idx ON public.algorithm_providers USING btree (project_id, status);


--
-- Name: algorithm_run_attempts_run_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX algorithm_run_attempts_run_idx ON public.algorithm_run_attempts USING btree (algorithm_run_id, attempt);


--
-- Name: algorithm_runs_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX algorithm_runs_claim_idx ON public.algorithm_runs USING btree (status, created_at);


--
-- Name: algorithm_runs_project_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX algorithm_runs_project_created_idx ON public.algorithm_runs USING btree (project_id, created_at DESC);


--
-- Name: algorithm_runs_task_step_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX algorithm_runs_task_step_idx ON public.algorithm_runs USING btree (task_run_step_id) WHERE (task_run_step_id IS NOT NULL);


--
-- Name: approval_requests_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX approval_requests_project_status_idx ON public.approval_requests USING btree (project_id, status, expires_at);


--
-- Name: approvals_request_decided_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX approvals_request_decided_idx ON public.approvals USING btree (approval_request_id, decided_at);


--
-- Name: asset_derivatives_source_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX asset_derivatives_source_idx ON public.asset_derivatives USING btree (project_id, source_asset_id);


--
-- Name: asset_upload_intents_expiry_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX asset_upload_intents_expiry_idx ON public.asset_upload_intents USING btree (status, expires_at);


--
-- Name: assets_device_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX assets_device_idx ON public.assets USING btree (device_id);


--
-- Name: assets_issue_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX assets_issue_idx ON public.assets USING btree (issue_id);


--
-- Name: assets_project_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX assets_project_created_idx ON public.assets USING btree (project_id, created_at);


--
-- Name: assets_project_status_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX assets_project_status_created_idx ON public.assets USING btree (project_id, status, created_at DESC);


--
-- Name: assets_retention_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX assets_retention_idx ON public.assets USING btree (project_id, retention_hold_until) WHERE (status = 'available'::text);


--
-- Name: assets_task_run_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX assets_task_run_idx ON public.assets USING btree (task_run_id);


--
-- Name: audit_events_project_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX audit_events_project_created_idx ON public.audit_events USING btree (project_id, created_at DESC);


--
-- Name: audit_events_request_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX audit_events_request_idx ON public.audit_events USING btree (request_id);


--
-- Name: audit_events_resource_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX audit_events_resource_idx ON public.audit_events USING btree (project_id, resource_type, resource_id);


--
-- Name: command_attempts_command_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX command_attempts_command_idx ON public.command_attempts USING btree (command_id, attempt);


--
-- Name: connector_action_jobs_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_action_jobs_pending_idx ON public.connector_action_jobs USING btree (connector_instance_id, action_kind, status, updated_at) WHERE (status <> ALL (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text]));


--
-- Name: connector_asset_access_refs_resource_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_asset_access_refs_resource_idx ON public.connector_asset_access_refs USING btree (project_id, connector_instance_id, remote_resource_id);


--
-- Name: connector_capability_snapshots_acceptance_scope_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_capability_snapshots_acceptance_scope_idx ON public.connector_capability_snapshots USING btree (connector_instance_id, account_fingerprint, capability_code, status, expires_at) WHERE (evidence_level = 'field-write'::text);


--
-- Name: connector_capability_snapshots_effective_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_capability_snapshots_effective_idx ON public.connector_capability_snapshots USING btree (connector_instance_id, status, expires_at);


--
-- Name: connector_capability_snapshots_identity_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX connector_capability_snapshots_identity_unique ON public.connector_capability_snapshots USING btree (project_id, connector_instance_id, capability_code, region, deployment, account_fingerprint, device_model, firmware_version) NULLS NOT DISTINCT;


--
-- Name: connector_control_sessions_device_exclusive_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX connector_control_sessions_device_exclusive_idx ON public.connector_control_sessions USING btree (project_id, device_id) WHERE (status = ANY (ARRAY['requested'::text, 'acquiring'::text, 'active'::text, 'releasing'::text]));


--
-- Name: connector_control_sessions_reconcile_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_control_sessions_reconcile_idx ON public.connector_control_sessions USING btree (status, lease_expires_at) WHERE (status = ANY (ARRAY['requested'::text, 'acquiring'::text, 'active'::text, 'releasing'::text]));


--
-- Name: connector_device_admin_jobs_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_device_admin_jobs_pending_idx ON public.connector_device_admin_jobs USING btree (status, updated_at) WHERE (status = ANY (ARRAY['queued'::text, 'executing'::text, 'accepted'::text]));


--
-- Name: connector_geospatial_action_jobs_active_target_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX connector_geospatial_action_jobs_active_target_unique ON public.connector_geospatial_action_jobs USING btree (target_resource_id) WHERE ((target_resource_id IS NOT NULL) AND (status = ANY (ARRAY['queued'::text, 'executing'::text])));


--
-- Name: connector_geospatial_action_jobs_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_geospatial_action_jobs_pending_idx ON public.connector_geospatial_action_jobs USING btree (connector_instance_id, action_kind, status, updated_at) WHERE (status = ANY (ARRAY['queued'::text, 'executing'::text]));


--
-- Name: connector_live_action_jobs_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_live_action_jobs_pending_idx ON public.connector_live_action_jobs USING btree (connector_instance_id, action_kind, status, updated_at) WHERE (status = ANY (ARRAY['queued'::text, 'executing'::text]));


--
-- Name: connector_management_write_jobs_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_management_write_jobs_pending_idx ON public.connector_management_write_jobs USING btree (status, updated_at) WHERE (status = ANY (ARRAY['queued'::text, 'executing'::text, 'accepted'::text]));


--
-- Name: connector_model_delete_jobs_active_target_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX connector_model_delete_jobs_active_target_unique ON public.connector_model_delete_jobs USING btree (target_resource_id) WHERE (status = ANY (ARRAY['queued'::text, 'executing'::text]));


--
-- Name: connector_model_delete_jobs_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_model_delete_jobs_pending_idx ON public.connector_model_delete_jobs USING btree (connector_instance_id, action_kind, status, updated_at) WHERE (status = ANY (ARRAY['queued'::text, 'executing'::text]));


--
-- Name: connector_model_jobs_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_model_jobs_pending_idx ON public.connector_model_jobs USING btree (connector_instance_id, status, updated_at) WHERE (status = ANY (ARRAY['queued'::text, 'reconciling'::text]));


--
-- Name: connector_object_upload_jobs_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_object_upload_jobs_pending_idx ON public.connector_object_upload_jobs USING btree (connector_instance_id, operation_kind, status, updated_at) WHERE (status <> ALL (ARRAY['succeeded'::text, 'failed'::text]));


--
-- Name: connector_open_model_uploads_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_open_model_uploads_pending_idx ON public.connector_open_model_uploads USING btree (connector_instance_id, status, updated_at) WHERE (status <> ALL (ARRAY['succeeded'::text, 'expired'::text, 'failed'::text, 'blocked'::text]));


--
-- Name: connector_remote_resources_canonical_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_remote_resources_canonical_idx ON public.connector_remote_resources USING btree (project_id, canonical_target_type, canonical_target_id) WHERE (canonical_target_type IS NOT NULL);


--
-- Name: connector_remote_resources_lookup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_remote_resources_lookup_idx ON public.connector_remote_resources USING btree (project_id, resource_kind, status, last_seen_at DESC);


--
-- Name: connector_resource_sync_states_due_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_resource_sync_states_due_idx ON public.connector_resource_sync_states USING btree (connector_instance_id, status, next_attempt_at);


--
-- Name: connector_sync_runs_connector_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_sync_runs_connector_created_idx ON public.connector_sync_runs USING btree (connector_instance_id, created_at DESC);


--
-- Name: coordinate_references_one_standard_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX coordinate_references_one_standard_idx ON public.coordinate_references USING btree (project_id) WHERE is_project_standard;


--
-- Name: detection_groups_geometry_gist; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX detection_groups_geometry_gist ON public.detection_groups USING gist (geographic_geometry);


--
-- Name: detection_groups_project_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX detection_groups_project_time_idx ON public.detection_groups USING btree (project_id, last_detected_at DESC);


--
-- Name: detections_geometry_gist; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX detections_geometry_gist ON public.detections USING gist (geographic_geometry);


--
-- Name: detections_project_captured_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX detections_project_captured_idx ON public.detections USING btree (project_id, captured_at DESC);


--
-- Name: device_adapters_connector_definition_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_adapters_connector_definition_idx ON public.device_adapters USING btree (connector_definition_id, status);


--
-- Name: device_adapters_connector_external_scope_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX device_adapters_connector_external_scope_unique ON public.device_adapters USING btree (project_id, connector_definition_id, external_scope_key) WHERE (external_scope_key IS NOT NULL);


--
-- Name: device_adapters_lease_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_adapters_lease_claim_idx ON public.device_adapters USING btree (adapter_type, status, lease_expires_at) WHERE (status = ANY (ARRAY['connecting'::text, 'connected'::text, 'degraded'::text]));


--
-- Name: device_adapters_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_adapters_project_status_idx ON public.device_adapters USING btree (project_id, status);


--
-- Name: device_capabilities_code_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_capabilities_code_idx ON public.device_capabilities USING btree (capability_code);


--
-- Name: device_capabilities_device_code_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX device_capabilities_device_code_unique ON public.device_capabilities USING btree (device_id, capability_code);


--
-- Name: device_capabilities_type_code_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_capabilities_type_code_idx ON public.device_capabilities USING btree (device_type_id, capability_code);


--
-- Name: device_capability_grants_lookup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_capability_grants_lookup_idx ON public.device_capability_grants USING btree (project_id, user_id, action_pattern, expires_at);


--
-- Name: device_capability_grants_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX device_capability_grants_unique ON public.device_capability_grants USING btree (project_id, user_id, scope_type, device_type_id, device_id, action_pattern) NULLS NOT DISTINCT;


--
-- Name: device_command_protocol_correlations_reply_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_command_protocol_correlations_reply_idx ON public.device_command_protocol_correlations USING btree (adapter_id, transaction_id, business_id, method, status);


--
-- Name: device_commands_dispatch_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_commands_dispatch_idx ON public.device_commands USING btree (status, priority DESC, deadline_at);


--
-- Name: device_commands_live_stream_action_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX device_commands_live_stream_action_unique ON public.device_commands USING btree (live_stream_id, command_key) WHERE (live_stream_id IS NOT NULL);


--
-- Name: device_commands_run_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_commands_run_created_idx ON public.device_commands USING btree (task_run_id, created_at);


--
-- Name: device_connections_device_opened_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_connections_device_opened_idx ON public.device_connections USING btree (device_id, opened_at DESC);


--
-- Name: device_connections_open_heartbeat_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_connections_open_heartbeat_idx ON public.device_connections USING btree (last_heartbeat_at) WHERE (closed_at IS NULL);


--
-- Name: device_connections_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_connections_project_status_idx ON public.device_connections USING btree (project_id, status);


--
-- Name: device_connector_bindings_connector_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_connector_bindings_connector_idx ON public.device_connector_bindings USING btree (connector_instance_id, status);


--
-- Name: device_connector_bindings_device_route_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_connector_bindings_device_route_idx ON public.device_connector_bindings USING btree (device_id, status, priority);


--
-- Name: device_external_identities_discovery_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_external_identities_discovery_idx ON public.device_external_identities USING btree (project_id, discovery_status, last_seen_at DESC);


--
-- Name: device_external_identities_project_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_external_identities_project_idx ON public.device_external_identities USING btree (project_id, last_seen_at DESC);


--
-- Name: device_latest_telemetry_project_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_latest_telemetry_project_time_idx ON public.device_latest_telemetry USING btree (project_id, captured_at DESC);


--
-- Name: device_protocol_messages_project_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_protocol_messages_project_time_idx ON public.device_protocol_messages USING btree (project_id, received_at DESC);


--
-- Name: device_relationships_from_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_relationships_from_idx ON public.device_relationships USING btree (from_device_id, relation_type, valid_from DESC);


--
-- Name: device_relationships_to_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_relationships_to_idx ON public.device_relationships USING btree (to_device_id, relation_type, valid_from DESC);


--
-- Name: device_stream_channels_project_type_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_stream_channels_project_type_idx ON public.device_stream_channels USING btree (project_id, data_type, availability);


--
-- Name: device_telemetry_source_event_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX device_telemetry_source_event_unique ON ONLY public.device_telemetry USING btree (adapter_id, event_id, captured_at);


--
-- Name: device_telemetry_default_adapter_id_event_id_captured_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX device_telemetry_default_adapter_id_event_id_captured_at_idx ON public.device_telemetry_default USING btree (adapter_id, event_id, captured_at);


--
-- Name: device_telemetry_device_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_telemetry_device_time_idx ON ONLY public.device_telemetry USING btree (device_id, captured_at DESC);


--
-- Name: device_telemetry_default_device_id_captured_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_telemetry_default_device_id_captured_at_idx ON public.device_telemetry_default USING btree (device_id, captured_at DESC);


--
-- Name: device_telemetry_project_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_telemetry_project_time_idx ON ONLY public.device_telemetry USING btree (project_id, captured_at DESC);


--
-- Name: device_telemetry_default_project_id_captured_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_telemetry_default_project_id_captured_at_idx ON public.device_telemetry_default USING btree (project_id, captured_at DESC);


--
-- Name: device_types_driver_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_types_driver_idx ON public.device_types USING btree (driver_definition_id, status, type_key);


--
-- Name: devices_id_project_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX devices_id_project_unique ON public.devices USING btree (id, project_id);


--
-- Name: devices_last_seen_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX devices_last_seen_idx ON public.devices USING btree (last_seen_at);


--
-- Name: devices_project_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX devices_project_name_unique ON public.devices USING btree (project_id, name);


--
-- Name: devices_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX devices_project_status_idx ON public.devices USING btree (project_id, status);


--
-- Name: devices_project_type_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX devices_project_type_idx ON public.devices USING btree (project_id, device_type_id);


--
-- Name: devices_registration_number_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX devices_registration_number_idx ON public.devices USING btree (uav_registration_number) WHERE (uav_registration_number IS NOT NULL);


--
-- Name: driver_definitions_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX driver_definitions_status_idx ON public.driver_definitions USING btree (status, driver_key);


--
-- Name: event_feedback_event_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX event_feedback_event_created_idx ON public.event_feedback USING btree (perception_event_id, created_at);


--
-- Name: event_rule_versions_one_draft_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX event_rule_versions_one_draft_idx ON public.event_rule_versions USING btree (event_rule_id) WHERE (status = 'draft'::text);


--
-- Name: evidence_links_target_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX evidence_links_target_idx ON public.evidence_links USING btree (project_id, target_type, target_id);


--
-- Name: generated_report_evidence_asset_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX generated_report_evidence_asset_idx ON public.generated_report_evidence USING btree (project_id, asset_id) WHERE (asset_id IS NOT NULL);


--
-- Name: generated_report_versions_one_draft_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX generated_report_versions_one_draft_idx ON public.generated_report_versions USING btree (generated_report_id) WHERE (status = 'draft'::text);


--
-- Name: generated_reports_project_updated_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX generated_reports_project_updated_idx ON public.generated_reports USING btree (project_id, updated_at DESC);


--
-- Name: idempotency_records_expiry_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idempotency_records_expiry_idx ON public.idempotency_records USING btree (expires_at);


--
-- Name: idempotency_records_project_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idempotency_records_project_created_idx ON public.idempotency_records USING btree (project_id, created_at DESC);


--
-- Name: issue_assignees_active_agent_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX issue_assignees_active_agent_unique ON public.issue_assignees USING btree (issue_id, agent_id) WHERE (active AND (agent_id IS NOT NULL));


--
-- Name: issue_assignees_active_user_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX issue_assignees_active_user_unique ON public.issue_assignees USING btree (issue_id, user_id) WHERE (active AND (user_id IS NOT NULL));


--
-- Name: issue_assignees_project_issue_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issue_assignees_project_issue_idx ON public.issue_assignees USING btree (project_id, issue_id) WHERE active;


--
-- Name: issue_events_client_key_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX issue_events_client_key_unique ON public.issue_events USING btree (project_id, issue_id, client_key) WHERE (client_key IS NOT NULL);


--
-- Name: issue_events_issue_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issue_events_issue_created_idx ON public.issue_events USING btree (issue_id, created_at);


--
-- Name: issue_events_project_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issue_events_project_created_idx ON public.issue_events USING btree (project_id, created_at);


--
-- Name: issue_feedback_quality_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issue_feedback_quality_idx ON public.issue_feedback USING btree (project_id, algorithm_definition_version_id, task_version_id, action, created_at DESC);


--
-- Name: issue_links_issue_target_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX issue_links_issue_target_unique ON public.issue_links USING btree (issue_id, link_type, target_id);


--
-- Name: issue_links_project_issue_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issue_links_project_issue_idx ON public.issue_links USING btree (project_id, issue_id);


--
-- Name: issue_links_target_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issue_links_target_idx ON public.issue_links USING btree (link_type, target_id);


--
-- Name: issues_project_number_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX issues_project_number_unique ON public.issues USING btree (project_id, number);


--
-- Name: issues_project_priority_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issues_project_priority_idx ON public.issues USING btree (project_id, priority);


--
-- Name: issues_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issues_project_status_idx ON public.issues USING btree (project_id, status);


--
-- Name: issues_task_business_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX issues_task_business_unique ON public.issues USING btree (project_id, task_version_id, condition_scope_key, business_object_key) WHERE ((task_version_id IS NOT NULL) AND (condition_scope_key IS NOT NULL) AND (business_object_key IS NOT NULL));


--
-- Name: issues_task_run_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX issues_task_run_idx ON public.issues USING btree (task_run_id);


--
-- Name: live_streams_device_started_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX live_streams_device_started_idx ON public.live_streams USING btree (device_id, started_at DESC);


--
-- Name: live_streams_expired_lease_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX live_streams_expired_lease_idx ON public.live_streams USING btree (lease_expires_at) WHERE (status = ANY (ARRAY['requested'::text, 'starting'::text, 'live'::text, 'degraded'::text, 'stopping'::text]));


--
-- Name: live_streams_flighthub_reconcile_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX live_streams_flighthub_reconcile_idx ON public.live_streams USING btree (adapter_id, status, updated_at) WHERE ((source_type = 'dji_flighthub'::text) AND (status = ANY (ARRAY['requested'::text, 'starting'::text, 'live'::text, 'degraded'::text, 'stopping'::text])));


--
-- Name: live_streams_ingest_ref_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX live_streams_ingest_ref_unique ON public.live_streams USING btree (ingest_ref) WHERE (ingest_ref IS NOT NULL);


--
-- Name: live_streams_one_active_device_key_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX live_streams_one_active_device_key_idx ON public.live_streams USING btree (project_id, device_id, stream_key) WHERE (status = ANY (ARRAY['requested'::text, 'starting'::text, 'live'::text, 'degraded'::text, 'stopping'::text]));


--
-- Name: live_streams_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX live_streams_project_status_idx ON public.live_streams USING btree (project_id, status, started_at DESC);


--
-- Name: observations_device_type_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX observations_device_type_time_idx ON public.observations USING btree (device_id, observation_type, captured_at DESC);


--
-- Name: observations_original_geometry_gist; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX observations_original_geometry_gist ON public.observations USING gist (original_geometry);


--
-- Name: observations_project_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX observations_project_time_idx ON public.observations USING btree (project_id, captured_at DESC, id);


--
-- Name: observations_standard_geometry_gist; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX observations_standard_geometry_gist ON public.observations USING gist (standard_geometry);


--
-- Name: observations_task_run_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX observations_task_run_time_idx ON public.observations USING btree (project_id, task_run_id, captured_at) WHERE (task_run_id IS NOT NULL);


--
-- Name: outbox_events_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX outbox_events_claim_idx ON public.outbox_events USING btree (status, available_at, locked_until, id);


--
-- Name: outbox_events_project_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX outbox_events_project_created_idx ON public.outbox_events USING btree (project_id, created_at, id);


--
-- Name: perception_events_active_dedup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX perception_events_active_dedup_idx ON public.perception_events USING btree (project_id, deduplication_key) WHERE (status = ANY (ARRAY['open'::text, 'acknowledged'::text, 'investigating'::text]));


--
-- Name: perception_events_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX perception_events_project_status_idx ON public.perception_events USING btree (project_id, status, last_detected_at DESC);


--
-- Name: platform_audit_events_actor_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX platform_audit_events_actor_created_idx ON public.platform_audit_events USING btree (actor_user_id, created_at DESC);


--
-- Name: poses_device_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX poses_device_time_idx ON public.poses USING btree (device_id, captured_at DESC, observation_id);


--
-- Name: poses_project_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX poses_project_time_idx ON public.poses USING btree (project_id, captured_at DESC, observation_id);


--
-- Name: poses_standard_position_gist; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX poses_standard_position_gist ON public.poses USING gist (standard_position);


--
-- Name: project_events_project_cursor_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX project_events_project_cursor_idx ON public.project_events USING btree (project_id, cursor);


--
-- Name: project_events_project_occurred_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX project_events_project_occurred_idx ON public.project_events USING btree (project_id, occurred_at, cursor);


--
-- Name: project_feature_flags_updated_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX project_feature_flags_updated_idx ON public.project_feature_flags USING btree (updated_at);


--
-- Name: project_permissions_project_permission_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX project_permissions_project_permission_idx ON public.project_permissions USING btree (project_id, permission);


--
-- Name: project_permissions_user_project_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX project_permissions_user_project_idx ON public.project_permissions USING btree (user_id, project_id);


--
-- Name: projects_id_team_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX projects_id_team_unique ON public.projects USING btree (id, team_id);


--
-- Name: projects_team_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX projects_team_idx ON public.projects USING btree (team_id);


--
-- Name: projects_team_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX projects_team_name_unique ON public.projects USING btree (team_id, name);


--
-- Name: retention_cleanup_runs_project_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX retention_cleanup_runs_project_created_idx ON public.retention_cleanup_runs USING btree (project_id, created_at DESC);


--
-- Name: retention_holds_one_active_asset_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX retention_holds_one_active_asset_idx ON public.retention_holds USING btree (project_id, asset_id) WHERE (status = 'active'::text);


--
-- Name: retention_policies_one_default_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX retention_policies_one_default_idx ON public.retention_policies USING btree (project_id) WHERE ((status = 'published'::text) AND is_default);


--
-- Name: retention_tombstones_project_deleted_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX retention_tombstones_project_deleted_idx ON public.retention_deletion_tombstones USING btree (project_id, deleted_at DESC);


--
-- Name: safety_policy_versions_boundary_gist; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX safety_policy_versions_boundary_gist ON public.safety_policy_versions USING gist (project_boundary);


--
-- Name: safety_policy_versions_one_draft_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX safety_policy_versions_one_draft_idx ON public.safety_policy_versions USING btree (project_id) WHERE (status = 'draft'::text);


--
-- Name: safety_policy_versions_restricted_gist; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX safety_policy_versions_restricted_gist ON public.safety_policy_versions USING gist (restricted_areas);


--
-- Name: sensor_calibrations_device_valid_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sensor_calibrations_device_valid_idx ON public.sensor_calibrations USING btree (device_id, sensor_key, valid_from DESC);


--
-- Name: sessions_expiry_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sessions_expiry_idx ON public.sessions USING btree (expiry);


--
-- Name: task_run_steps_run_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX task_run_steps_run_status_idx ON public.task_run_steps USING btree (task_run_id, status, "position");


--
-- Name: task_runs_active_concurrency_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX task_runs_active_concurrency_idx ON public.task_runs USING btree (project_id, task_version_id, status) WHERE (status = ANY (ARRAY['queued'::text, 'blocked'::text, 'ready'::text, 'dispatching'::text, 'running'::text, 'paused'::text, 'canceling'::text]));


--
-- Name: task_runs_project_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX task_runs_project_created_idx ON public.task_runs USING btree (project_id, created_at);


--
-- Name: task_runs_responsible_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX task_runs_responsible_created_idx ON public.task_runs USING btree (responsible_user_id, created_at DESC) WHERE (responsible_user_id IS NOT NULL);


--
-- Name: task_runs_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX task_runs_status_idx ON public.task_runs USING btree (status);


--
-- Name: task_runs_task_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX task_runs_task_created_idx ON public.task_runs USING btree (task_id, created_at);


--
-- Name: task_runs_trigger_key_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX task_runs_trigger_key_unique ON public.task_runs USING btree (project_id, task_version_id, trigger_key) WHERE ((task_version_id IS NOT NULL) AND (trigger_key IS NOT NULL));


--
-- Name: task_steps_version_position_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX task_steps_version_position_idx ON public.task_steps USING btree (task_version_id, "position");


--
-- Name: task_versions_one_draft_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX task_versions_one_draft_idx ON public.task_versions USING btree (task_id) WHERE (status = 'draft'::text);


--
-- Name: task_versions_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX task_versions_project_status_idx ON public.task_versions USING btree (project_id, status, created_at DESC);


--
-- Name: tasks_project_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX tasks_project_name_unique ON public.tasks USING btree (project_id, name);


--
-- Name: tasks_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tasks_project_status_idx ON public.tasks USING btree (project_id, status);


--
-- Name: tasks_trigger_type_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tasks_trigger_type_idx ON public.tasks USING btree (trigger_type);


--
-- Name: team_members_single_owner_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX team_members_single_owner_unique ON public.team_members USING btree (team_id) WHERE (role = 'owner'::text);


--
-- Name: team_members_team_user_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX team_members_team_user_unique ON public.team_members USING btree (team_id, user_id);


--
-- Name: team_members_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX team_members_user_idx ON public.team_members USING btree (user_id);


--
-- Name: telemetry_event_dedup_received_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX telemetry_event_dedup_received_idx ON public.telemetry_event_dedup USING btree (received_at);


--
-- Name: device_telemetry_default_adapter_id_event_id_captured_at_idx; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.device_telemetry_source_event_unique ATTACH PARTITION public.device_telemetry_default_adapter_id_event_id_captured_at_idx;


--
-- Name: device_telemetry_default_device_id_captured_at_idx; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.device_telemetry_device_time_idx ATTACH PARTITION public.device_telemetry_default_device_id_captured_at_idx;


--
-- Name: device_telemetry_default_pkey; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.device_telemetry_pkey ATTACH PARTITION public.device_telemetry_default_pkey;


--
-- Name: device_telemetry_default_project_id_captured_at_idx; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.device_telemetry_project_time_idx ATTACH PARTITION public.device_telemetry_default_project_id_captured_at_idx;


--
-- Name: algorithm_definition_versions algorithm_definition_versions_published_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER algorithm_definition_versions_published_immutable BEFORE DELETE OR UPDATE ON public.algorithm_definition_versions FOR EACH ROW EXECUTE FUNCTION public.protect_published_algorithm_definition_version();


--
-- Name: approvals approvals_project_request_status; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER approvals_project_request_status AFTER INSERT ON public.approvals FOR EACH ROW EXECUTE FUNCTION public.project_approval_request_status();


--
-- Name: approvals approvals_validate_decision; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER approvals_validate_decision BEFORE INSERT ON public.approvals FOR EACH ROW EXECUTE FUNCTION public.validate_approval_decision();


--
-- Name: device_adapters device_adapters_populate_connector_definition; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER device_adapters_populate_connector_definition BEFORE INSERT OR UPDATE OF adapter_type, connector_definition_id ON public.device_adapters FOR EACH ROW EXECUTE FUNCTION public.populate_device_adapter_connector_definition();


--
-- Name: device_capabilities device_capabilities_populate_type_driver; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER device_capabilities_populate_type_driver BEFORE INSERT OR UPDATE OF device_id, project_id, device_type_id, driver_definition_id ON public.device_capabilities FOR EACH ROW EXECUTE FUNCTION public.populate_device_capability_type_driver();


--
-- Name: device_stream_channels device_stream_channels_populate_stable_id; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER device_stream_channels_populate_stable_id BEFORE INSERT OR UPDATE OF project_id, device_id, channel_key, stable_channel_id ON public.device_stream_channels FOR EACH ROW EXECUTE FUNCTION public.populate_device_stream_channel_stable_id();


--
-- Name: event_rule_versions event_rule_versions_published_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER event_rule_versions_published_immutable BEFORE DELETE OR UPDATE ON public.event_rule_versions FOR EACH ROW EXECUTE FUNCTION public.protect_published_event_rule_version();


--
-- Name: evidence_links evidence_links_published_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER evidence_links_published_immutable BEFORE DELETE OR UPDATE ON public.evidence_links FOR EACH ROW EXECUTE FUNCTION public.protect_published_evidence_link();


--
-- Name: outbox_events outbox_events_notify; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER outbox_events_notify AFTER INSERT ON public.outbox_events FOR EACH ROW EXECUTE FUNCTION public.notify_aerosight_outbox();


--
-- Name: project_events project_events_notify; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER project_events_notify AFTER INSERT ON public.project_events FOR EACH ROW EXECUTE FUNCTION public.notify_aerosight_project_event();


--
-- Name: projects projects_provision_copilot_agent; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER projects_provision_copilot_agent AFTER INSERT ON public.projects FOR EACH ROW EXECUTE FUNCTION public.provision_project_copilot_agent();


--
-- Name: retention_policies retention_policies_published_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER retention_policies_published_immutable BEFORE DELETE OR UPDATE ON public.retention_policies FOR EACH ROW EXECUTE FUNCTION public.protect_published_retention_policy();


--
-- Name: safety_policy_versions safety_policy_versions_published_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER safety_policy_versions_published_immutable BEFORE DELETE OR UPDATE ON public.safety_policy_versions FOR EACH ROW EXECUTE FUNCTION public.protect_published_safety_policy_version();


--
-- Name: task_steps task_steps_published_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER task_steps_published_immutable BEFORE DELETE OR UPDATE ON public.task_steps FOR EACH ROW EXECUTE FUNCTION public.protect_published_task_step();


--
-- Name: task_versions task_versions_published_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER task_versions_published_immutable BEFORE DELETE OR UPDATE ON public.task_versions FOR EACH ROW EXECUTE FUNCTION public.protect_published_task_version();


--
-- Name: agent_draft_evidence agent_draft_evidence_draft_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_draft_evidence
    ADD CONSTRAINT agent_draft_evidence_draft_fk FOREIGN KEY (agent_draft_id, project_id) REFERENCES public.agent_drafts(id, project_id) ON DELETE CASCADE;


--
-- Name: agent_drafts agent_drafts_actor_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_drafts
    ADD CONSTRAINT agent_drafts_actor_team_fk FOREIGN KEY (team_id, created_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: agent_drafts agent_drafts_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_drafts
    ADD CONSTRAINT agent_drafts_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: agent_drafts agent_drafts_session_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_drafts
    ADD CONSTRAINT agent_drafts_session_project_fk FOREIGN KEY (session_id, project_id) REFERENCES public.agent_sessions(id, project_id) ON DELETE CASCADE;


--
-- Name: agent_messages agent_messages_session_id_agent_sessions_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_messages
    ADD CONSTRAINT agent_messages_session_id_agent_sessions_id_fk FOREIGN KEY (session_id) REFERENCES public.agent_sessions(id) ON DELETE CASCADE;


--
-- Name: agent_sessions agent_sessions_agent_id_agents_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_sessions
    ADD CONSTRAINT agent_sessions_agent_id_agents_id_fk FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;


--
-- Name: agent_sessions agent_sessions_agent_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_sessions
    ADD CONSTRAINT agent_sessions_agent_project_fk FOREIGN KEY (agent_id, project_id) REFERENCES public.agents(id, project_id) ON DELETE SET NULL (agent_id);


--
-- Name: agent_sessions agent_sessions_issue_id_issues_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_sessions
    ADD CONSTRAINT agent_sessions_issue_id_issues_id_fk FOREIGN KEY (issue_id) REFERENCES public.issues(id) ON DELETE SET NULL;


--
-- Name: agent_sessions agent_sessions_issue_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_sessions
    ADD CONSTRAINT agent_sessions_issue_project_fk FOREIGN KEY (issue_id, project_id) REFERENCES public.issues(id, project_id) ON DELETE SET NULL (issue_id);


--
-- Name: agent_sessions agent_sessions_project_id_projects_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_sessions
    ADD CONSTRAINT agent_sessions_project_id_projects_id_fk FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: agent_sessions agent_sessions_started_by_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_sessions
    ADD CONSTRAINT agent_sessions_started_by_user_id_users_id_fk FOREIGN KEY (started_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: agent_sessions agent_sessions_task_run_id_task_runs_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_sessions
    ADD CONSTRAINT agent_sessions_task_run_id_task_runs_id_fk FOREIGN KEY (task_run_id) REFERENCES public.task_runs(id) ON DELETE SET NULL;


--
-- Name: agent_tool_jobs agent_tool_jobs_issue_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_tool_jobs
    ADD CONSTRAINT agent_tool_jobs_issue_project_fk FOREIGN KEY (issue_id, project_id) REFERENCES public.issues(id, project_id) ON DELETE CASCADE;


--
-- Name: agent_tool_jobs agent_tool_jobs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_tool_jobs
    ADD CONSTRAINT agent_tool_jobs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: agent_tool_jobs agent_tool_jobs_session_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_tool_jobs
    ADD CONSTRAINT agent_tool_jobs_session_project_fk FOREIGN KEY (session_id, project_id) REFERENCES public.agent_sessions(id, project_id) ON DELETE CASCADE;


--
-- Name: agent_tool_jobs agent_tool_jobs_trigger_event_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_tool_jobs
    ADD CONSTRAINT agent_tool_jobs_trigger_event_project_fk FOREIGN KEY (trigger_issue_event_id, project_id) REFERENCES public.issue_events(id, project_id) ON DELETE CASCADE;


--
-- Name: agents agents_project_id_projects_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_project_id_projects_id_fk FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: ai_providers ai_providers_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_providers
    ADD CONSTRAINT ai_providers_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: ai_providers ai_providers_updated_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_providers
    ADD CONSTRAINT ai_providers_updated_by_user_id_fkey FOREIGN KEY (updated_by_user_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: alert_automation_drafts alert_automation_drafts_event_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_drafts
    ADD CONSTRAINT alert_automation_drafts_event_project_fk FOREIGN KEY (perception_event_id, project_id) REFERENCES public.perception_events(id, project_id) ON DELETE RESTRICT;


--
-- Name: alert_automation_drafts alert_automation_drafts_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_drafts
    ADD CONSTRAINT alert_automation_drafts_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: alert_automation_drafts alert_automation_drafts_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_drafts
    ADD CONSTRAINT alert_automation_drafts_run_project_fk FOREIGN KEY (automation_run_id, project_id) REFERENCES public.alert_automation_runs(id, project_id) ON DELETE CASCADE;


--
-- Name: alert_automation_policies alert_automation_policies_current_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policies
    ADD CONSTRAINT alert_automation_policies_current_version_project_fk FOREIGN KEY (current_published_version_id, project_id) REFERENCES public.alert_automation_policy_versions(id, project_id) ON DELETE SET NULL;


--
-- Name: alert_automation_policies alert_automation_policies_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policies
    ADD CONSTRAINT alert_automation_policies_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: alert_automation_policy_versions alert_automation_policy_versions_policy_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policy_versions
    ADD CONSTRAINT alert_automation_policy_versions_policy_project_fk FOREIGN KEY (alert_automation_policy_id, project_id) REFERENCES public.alert_automation_policies(id, project_id) ON DELETE CASCADE;


--
-- Name: alert_automation_policy_versions alert_automation_policy_versions_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policy_versions
    ADD CONSTRAINT alert_automation_policy_versions_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: alert_automation_policy_versions alert_automation_policy_versions_rule_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_policy_versions
    ADD CONSTRAINT alert_automation_policy_versions_rule_project_fk FOREIGN KEY (event_rule_version_id, project_id) REFERENCES public.event_rule_versions(id, project_id) ON DELETE RESTRICT;


--
-- Name: alert_automation_runs alert_automation_runs_event_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_runs
    ADD CONSTRAINT alert_automation_runs_event_project_fk FOREIGN KEY (perception_event_id, project_id) REFERENCES public.perception_events(id, project_id) ON DELETE RESTRICT;


--
-- Name: alert_automation_runs alert_automation_runs_policy_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_runs
    ADD CONSTRAINT alert_automation_runs_policy_project_fk FOREIGN KEY (policy_version_id, project_id) REFERENCES public.alert_automation_policy_versions(id, project_id) ON DELETE RESTRICT;


--
-- Name: alert_automation_runs alert_automation_runs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_automation_runs
    ADD CONSTRAINT alert_automation_runs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: algorithm_callback_receipts algorithm_callback_receipts_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_callback_receipts
    ADD CONSTRAINT algorithm_callback_receipts_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: algorithm_callback_receipts algorithm_callback_receipts_provider_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_callback_receipts
    ADD CONSTRAINT algorithm_callback_receipts_provider_project_fk FOREIGN KEY (provider_id, project_id) REFERENCES public.algorithm_providers(id, project_id) ON DELETE CASCADE;


--
-- Name: algorithm_callback_receipts algorithm_callback_receipts_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_callback_receipts
    ADD CONSTRAINT algorithm_callback_receipts_run_project_fk FOREIGN KEY (algorithm_run_id, project_id) REFERENCES public.algorithm_runs(id, project_id) ON DELETE CASCADE;


--
-- Name: algorithm_definition_versions algorithm_definition_versions_creator_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definition_versions
    ADD CONSTRAINT algorithm_definition_versions_creator_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: algorithm_definition_versions algorithm_definition_versions_definition_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definition_versions
    ADD CONSTRAINT algorithm_definition_versions_definition_project_fk FOREIGN KEY (algorithm_definition_id, project_id) REFERENCES public.algorithm_definitions(id, project_id) ON DELETE CASCADE;


--
-- Name: algorithm_definition_versions algorithm_definition_versions_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definition_versions
    ADD CONSTRAINT algorithm_definition_versions_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: algorithm_definition_versions algorithm_definition_versions_publisher_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definition_versions
    ADD CONSTRAINT algorithm_definition_versions_publisher_fk FOREIGN KEY (published_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: algorithm_definitions algorithm_definitions_creator_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definitions
    ADD CONSTRAINT algorithm_definitions_creator_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: algorithm_definitions algorithm_definitions_current_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definitions
    ADD CONSTRAINT algorithm_definitions_current_version_project_fk FOREIGN KEY (current_published_version_id, project_id) REFERENCES public.algorithm_definition_versions(id, project_id) ON DELETE SET NULL (current_published_version_id);


--
-- Name: algorithm_definitions algorithm_definitions_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definitions
    ADD CONSTRAINT algorithm_definitions_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: algorithm_definitions algorithm_definitions_provider_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_definitions
    ADD CONSTRAINT algorithm_definitions_provider_project_fk FOREIGN KEY (provider_id, project_id) REFERENCES public.algorithm_providers(id, project_id) ON DELETE CASCADE;


--
-- Name: algorithm_providers algorithm_providers_creator_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_providers
    ADD CONSTRAINT algorithm_providers_creator_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: algorithm_providers algorithm_providers_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_providers
    ADD CONSTRAINT algorithm_providers_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: algorithm_run_attempts algorithm_run_attempts_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_run_attempts
    ADD CONSTRAINT algorithm_run_attempts_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: algorithm_run_attempts algorithm_run_attempts_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_run_attempts
    ADD CONSTRAINT algorithm_run_attempts_run_project_fk FOREIGN KEY (algorithm_run_id, project_id) REFERENCES public.algorithm_runs(id, project_id) ON DELETE CASCADE;


--
-- Name: algorithm_runs algorithm_runs_asset_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_runs
    ADD CONSTRAINT algorithm_runs_asset_project_fk FOREIGN KEY (input_asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE RESTRICT;


--
-- Name: algorithm_runs algorithm_runs_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_runs
    ADD CONSTRAINT algorithm_runs_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE SET NULL (device_id);


--
-- Name: algorithm_runs algorithm_runs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_runs
    ADD CONSTRAINT algorithm_runs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: algorithm_runs algorithm_runs_task_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_runs
    ADD CONSTRAINT algorithm_runs_task_run_project_fk FOREIGN KEY (task_run_id, project_id) REFERENCES public.task_runs(id, project_id) ON DELETE SET NULL (task_run_id);


--
-- Name: algorithm_runs algorithm_runs_task_step_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_runs
    ADD CONSTRAINT algorithm_runs_task_step_project_fk FOREIGN KEY (task_run_step_id, project_id) REFERENCES public.task_run_steps(id, project_id) ON DELETE SET NULL (task_run_step_id);


--
-- Name: algorithm_runs algorithm_runs_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.algorithm_runs
    ADD CONSTRAINT algorithm_runs_version_project_fk FOREIGN KEY (algorithm_definition_version_id, project_id) REFERENCES public.algorithm_definition_versions(id, project_id) ON DELETE RESTRICT;


--
-- Name: approval_requests approval_requests_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: approval_requests approval_requests_requester_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_requester_member_fk FOREIGN KEY (team_id, requested_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: approvals approvals_approver_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approvals
    ADD CONSTRAINT approvals_approver_member_fk FOREIGN KEY (team_id, approver_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: approvals approvals_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approvals
    ADD CONSTRAINT approvals_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: approvals approvals_request_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approvals
    ADD CONSTRAINT approvals_request_project_fk FOREIGN KEY (approval_request_id, project_id) REFERENCES public.approval_requests(id, project_id) ON DELETE CASCADE;


--
-- Name: asset_derivatives asset_derivatives_derived_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_derivatives
    ADD CONSTRAINT asset_derivatives_derived_project_fk FOREIGN KEY (derived_asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE CASCADE;


--
-- Name: asset_derivatives asset_derivatives_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_derivatives
    ADD CONSTRAINT asset_derivatives_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: asset_derivatives asset_derivatives_source_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_derivatives
    ADD CONSTRAINT asset_derivatives_source_project_fk FOREIGN KEY (source_asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE CASCADE;


--
-- Name: asset_upload_intents asset_upload_intents_actor_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_upload_intents
    ADD CONSTRAINT asset_upload_intents_actor_fk FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: asset_upload_intents asset_upload_intents_asset_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_upload_intents
    ADD CONSTRAINT asset_upload_intents_asset_project_fk FOREIGN KEY (asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE SET NULL (asset_id);


--
-- Name: asset_upload_intents asset_upload_intents_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_upload_intents
    ADD CONSTRAINT asset_upload_intents_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE SET NULL (device_id);


--
-- Name: asset_upload_intents asset_upload_intents_issue_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_upload_intents
    ADD CONSTRAINT asset_upload_intents_issue_project_fk FOREIGN KEY (issue_id, project_id) REFERENCES public.issues(id, project_id) ON DELETE SET NULL (issue_id);


--
-- Name: asset_upload_intents asset_upload_intents_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_upload_intents
    ADD CONSTRAINT asset_upload_intents_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: asset_upload_intents asset_upload_intents_task_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_upload_intents
    ADD CONSTRAINT asset_upload_intents_task_run_project_fk FOREIGN KEY (task_run_id, project_id) REFERENCES public.task_runs(id, project_id) ON DELETE SET NULL (task_run_id);


--
-- Name: assets assets_device_id_devices_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_device_id_devices_id_fk FOREIGN KEY (device_id) REFERENCES public.devices(id) ON DELETE SET NULL;


--
-- Name: assets assets_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE SET NULL (device_id);


--
-- Name: assets assets_issue_id_issues_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_issue_id_issues_id_fk FOREIGN KEY (issue_id) REFERENCES public.issues(id) ON DELETE SET NULL;


--
-- Name: assets assets_issue_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_issue_project_fk FOREIGN KEY (issue_id, project_id) REFERENCES public.issues(id, project_id) ON DELETE SET NULL (issue_id);


--
-- Name: assets assets_project_id_projects_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_project_id_projects_id_fk FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: assets assets_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: assets assets_supersedes_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_supersedes_project_fk FOREIGN KEY (supersedes_asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE SET NULL (supersedes_asset_id);


--
-- Name: assets assets_task_run_id_task_runs_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_task_run_id_task_runs_id_fk FOREIGN KEY (task_run_id) REFERENCES public.task_runs(id) ON DELETE SET NULL;


--
-- Name: assets assets_task_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_task_run_project_fk FOREIGN KEY (task_run_id, project_id) REFERENCES public.task_runs(id, project_id) ON DELETE SET NULL (task_run_id);


--
-- Name: audit_events audit_events_actor_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_actor_agent_id_fkey FOREIGN KEY (actor_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;


--
-- Name: audit_events audit_events_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: audit_events audit_events_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: command_attempts command_attempts_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.command_attempts
    ADD CONSTRAINT command_attempts_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE RESTRICT;


--
-- Name: command_attempts command_attempts_command_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.command_attempts
    ADD CONSTRAINT command_attempts_command_project_fk FOREIGN KEY (command_id, project_id) REFERENCES public.device_commands(id, project_id) ON DELETE CASCADE;


--
-- Name: command_attempts command_attempts_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.command_attempts
    ADD CONSTRAINT command_attempts_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_action_jobs connector_action_jobs_approval_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_approval_project_fk FOREIGN KEY (approval_request_id, project_id) REFERENCES public.approval_requests(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_action_jobs connector_action_jobs_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_action_jobs connector_action_jobs_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_action_jobs connector_action_jobs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_action_jobs connector_action_jobs_requester_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_requester_member_fk FOREIGN KEY (team_id, requested_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: connector_action_jobs connector_action_jobs_result_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_result_project_fk FOREIGN KEY (remote_result_resource_id, project_id) REFERENCES public.connector_remote_resources(id, project_id) ON DELETE SET NULL (remote_result_resource_id);


--
-- Name: connector_action_jobs connector_action_jobs_target_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_target_project_fk FOREIGN KEY (target_resource_id, project_id) REFERENCES public.connector_remote_resources(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_action_jobs connector_action_jobs_task_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_task_run_project_fk FOREIGN KEY (task_run_id, project_id) REFERENCES public.task_runs(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_action_jobs connector_action_jobs_wayline_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_action_jobs
    ADD CONSTRAINT connector_action_jobs_wayline_project_fk FOREIGN KEY (wayline_resource_id, project_id) REFERENCES public.connector_remote_resources(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_asset_access_refs connector_asset_access_refs_asset_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_asset_access_refs
    ADD CONSTRAINT connector_asset_access_refs_asset_project_fk FOREIGN KEY (id, project_id) REFERENCES public.assets(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_asset_access_refs connector_asset_access_refs_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_asset_access_refs
    ADD CONSTRAINT connector_asset_access_refs_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_asset_access_refs connector_asset_access_refs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_asset_access_refs
    ADD CONSTRAINT connector_asset_access_refs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_asset_access_refs connector_asset_access_refs_resource_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_asset_access_refs
    ADD CONSTRAINT connector_asset_access_refs_resource_project_fk FOREIGN KEY (remote_resource_id, project_id) REFERENCES public.connector_remote_resources(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_capability_snapshots connector_capability_snapshots_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_capability_snapshots
    ADD CONSTRAINT connector_capability_snapshots_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_capability_snapshots connector_capability_snapshots_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_capability_snapshots
    ADD CONSTRAINT connector_capability_snapshots_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_control_sessions connector_control_sessions_approval_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_sessions
    ADD CONSTRAINT connector_control_sessions_approval_project_fk FOREIGN KEY (approval_request_id, project_id) REFERENCES public.approval_requests(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_control_sessions connector_control_sessions_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_sessions
    ADD CONSTRAINT connector_control_sessions_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_control_sessions connector_control_sessions_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_sessions
    ADD CONSTRAINT connector_control_sessions_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_control_sessions connector_control_sessions_holder_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_sessions
    ADD CONSTRAINT connector_control_sessions_holder_member_fk FOREIGN KEY (team_id, holder_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: connector_control_sessions connector_control_sessions_policy_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_sessions
    ADD CONSTRAINT connector_control_sessions_policy_project_fk FOREIGN KEY (safety_policy_version_id, project_id) REFERENCES public.safety_policy_versions(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_control_sessions connector_control_sessions_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_sessions
    ADD CONSTRAINT connector_control_sessions_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_device_admin_jobs connector_device_admin_jobs_approval_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_device_admin_jobs
    ADD CONSTRAINT connector_device_admin_jobs_approval_project_fk FOREIGN KEY (approval_request_id, project_id) REFERENCES public.approval_requests(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_device_admin_jobs connector_device_admin_jobs_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_device_admin_jobs
    ADD CONSTRAINT connector_device_admin_jobs_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_device_admin_jobs connector_device_admin_jobs_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_device_admin_jobs
    ADD CONSTRAINT connector_device_admin_jobs_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_device_admin_jobs connector_device_admin_jobs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_device_admin_jobs
    ADD CONSTRAINT connector_device_admin_jobs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_device_admin_jobs connector_device_admin_jobs_requester_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_device_admin_jobs
    ADD CONSTRAINT connector_device_admin_jobs_requester_member_fk FOREIGN KEY (team_id, requested_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: connector_geospatial_action_jobs connector_geospatial_action_jobs_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_geospatial_action_jobs
    ADD CONSTRAINT connector_geospatial_action_jobs_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_geospatial_action_jobs connector_geospatial_action_jobs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_geospatial_action_jobs
    ADD CONSTRAINT connector_geospatial_action_jobs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_geospatial_action_jobs connector_geospatial_action_jobs_requester_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_geospatial_action_jobs
    ADD CONSTRAINT connector_geospatial_action_jobs_requester_member_fk FOREIGN KEY (team_id, requested_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: connector_geospatial_action_jobs connector_geospatial_action_jobs_target_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_geospatial_action_jobs
    ADD CONSTRAINT connector_geospatial_action_jobs_target_project_fk FOREIGN KEY (target_resource_id, project_id) REFERENCES public.connector_remote_resources(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_live_action_jobs connector_live_action_jobs_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_live_action_jobs
    ADD CONSTRAINT connector_live_action_jobs_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_live_action_jobs connector_live_action_jobs_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_live_action_jobs
    ADD CONSTRAINT connector_live_action_jobs_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_live_action_jobs connector_live_action_jobs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_live_action_jobs
    ADD CONSTRAINT connector_live_action_jobs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_live_action_jobs connector_live_action_jobs_requester_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_live_action_jobs
    ADD CONSTRAINT connector_live_action_jobs_requester_member_fk FOREIGN KEY (team_id, requested_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: connector_live_action_jobs connector_live_action_jobs_target_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_live_action_jobs
    ADD CONSTRAINT connector_live_action_jobs_target_project_fk FOREIGN KEY (target_resource_id, project_id) REFERENCES public.connector_remote_resources(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_management_write_jobs connector_management_write_jobs_approval_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_management_write_jobs
    ADD CONSTRAINT connector_management_write_jobs_approval_project_fk FOREIGN KEY (approval_request_id, project_id) REFERENCES public.approval_requests(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_management_write_jobs connector_management_write_jobs_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_management_write_jobs
    ADD CONSTRAINT connector_management_write_jobs_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_management_write_jobs connector_management_write_jobs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_management_write_jobs
    ADD CONSTRAINT connector_management_write_jobs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_management_write_jobs connector_management_write_jobs_requester_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_management_write_jobs
    ADD CONSTRAINT connector_management_write_jobs_requester_member_fk FOREIGN KEY (team_id, requested_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: connector_model_delete_jobs connector_model_delete_jobs_approval_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_delete_jobs
    ADD CONSTRAINT connector_model_delete_jobs_approval_project_fk FOREIGN KEY (approval_request_id, project_id) REFERENCES public.approval_requests(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_model_delete_jobs connector_model_delete_jobs_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_delete_jobs
    ADD CONSTRAINT connector_model_delete_jobs_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_model_delete_jobs connector_model_delete_jobs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_delete_jobs
    ADD CONSTRAINT connector_model_delete_jobs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_model_delete_jobs connector_model_delete_jobs_requester_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_delete_jobs
    ADD CONSTRAINT connector_model_delete_jobs_requester_member_fk FOREIGN KEY (team_id, requested_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: connector_model_delete_jobs connector_model_delete_jobs_target_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_delete_jobs
    ADD CONSTRAINT connector_model_delete_jobs_target_project_fk FOREIGN KEY (target_resource_id, project_id) REFERENCES public.connector_remote_resources(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_model_jobs connector_model_jobs_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_jobs
    ADD CONSTRAINT connector_model_jobs_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_model_jobs connector_model_jobs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_jobs
    ADD CONSTRAINT connector_model_jobs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_model_jobs connector_model_jobs_requester_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_model_jobs
    ADD CONSTRAINT connector_model_jobs_requester_member_fk FOREIGN KEY (team_id, requested_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: connector_object_upload_jobs connector_object_upload_jobs_asset_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_object_upload_jobs
    ADD CONSTRAINT connector_object_upload_jobs_asset_project_fk FOREIGN KEY (source_asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE RESTRICT;


--
-- Name: connector_object_upload_jobs connector_object_upload_jobs_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_object_upload_jobs
    ADD CONSTRAINT connector_object_upload_jobs_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_object_upload_jobs connector_object_upload_jobs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_object_upload_jobs
    ADD CONSTRAINT connector_object_upload_jobs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_object_upload_jobs connector_object_upload_jobs_remote_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_object_upload_jobs
    ADD CONSTRAINT connector_object_upload_jobs_remote_project_fk FOREIGN KEY (remote_resource_id, project_id) REFERENCES public.connector_remote_resources(id, project_id) ON DELETE SET NULL (remote_resource_id);


--
-- Name: connector_object_upload_jobs connector_object_upload_jobs_requester_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_object_upload_jobs
    ADD CONSTRAINT connector_object_upload_jobs_requester_member_fk FOREIGN KEY (team_id, requested_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: connector_open_model_uploads connector_open_model_uploads_asset_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_open_model_uploads
    ADD CONSTRAINT connector_open_model_uploads_asset_project_fk FOREIGN KEY (asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE SET NULL (asset_id);


--
-- Name: connector_open_model_uploads connector_open_model_uploads_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_open_model_uploads
    ADD CONSTRAINT connector_open_model_uploads_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_open_model_uploads connector_open_model_uploads_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_open_model_uploads
    ADD CONSTRAINT connector_open_model_uploads_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_open_model_uploads connector_open_model_uploads_remote_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_open_model_uploads
    ADD CONSTRAINT connector_open_model_uploads_remote_project_fk FOREIGN KEY (remote_resource_id, project_id) REFERENCES public.connector_remote_resources(id, project_id) ON DELETE SET NULL (remote_resource_id);


--
-- Name: connector_open_model_uploads connector_open_model_uploads_requester_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_open_model_uploads
    ADD CONSTRAINT connector_open_model_uploads_requester_member_fk FOREIGN KEY (team_id, requested_by_user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE RESTRICT;


--
-- Name: connector_remote_resources connector_remote_resources_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_remote_resources
    ADD CONSTRAINT connector_remote_resources_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_remote_resources connector_remote_resources_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_remote_resources
    ADD CONSTRAINT connector_remote_resources_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_resource_sync_states connector_resource_sync_states_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_resource_sync_states
    ADD CONSTRAINT connector_resource_sync_states_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_resource_sync_states connector_resource_sync_states_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_resource_sync_states
    ADD CONSTRAINT connector_resource_sync_states_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: connector_sync_runs connector_sync_runs_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_sync_runs
    ADD CONSTRAINT connector_sync_runs_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: connector_sync_runs connector_sync_runs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_sync_runs
    ADD CONSTRAINT connector_sync_runs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: coordinate_references coordinate_references_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coordinate_references
    ADD CONSTRAINT coordinate_references_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: detection_group_members detection_group_members_detection_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detection_group_members
    ADD CONSTRAINT detection_group_members_detection_project_fk FOREIGN KEY (detection_id, project_id) REFERENCES public.detections(id, project_id) ON DELETE CASCADE;


--
-- Name: detection_group_members detection_group_members_group_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detection_group_members
    ADD CONSTRAINT detection_group_members_group_project_fk FOREIGN KEY (detection_group_id, project_id) REFERENCES public.detection_groups(id, project_id) ON DELETE CASCADE;


--
-- Name: detection_group_members detection_group_members_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detection_group_members
    ADD CONSTRAINT detection_group_members_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: detection_groups detection_groups_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detection_groups
    ADD CONSTRAINT detection_groups_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: detections detections_asset_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detections
    ADD CONSTRAINT detections_asset_project_fk FOREIGN KEY (input_asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE RESTRICT;


--
-- Name: detections detections_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detections
    ADD CONSTRAINT detections_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: detections detections_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detections
    ADD CONSTRAINT detections_run_project_fk FOREIGN KEY (algorithm_run_id, project_id) REFERENCES public.algorithm_runs(id, project_id) ON DELETE CASCADE;


--
-- Name: detections detections_task_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detections
    ADD CONSTRAINT detections_task_run_project_fk FOREIGN KEY (task_run_id, project_id) REFERENCES public.task_runs(id, project_id) ON DELETE SET NULL (task_run_id);


--
-- Name: device_adapters device_adapters_connector_definition_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_adapters
    ADD CONSTRAINT device_adapters_connector_definition_fk FOREIGN KEY (connector_definition_id) REFERENCES public.connector_definitions(id) ON DELETE RESTRICT;


--
-- Name: device_adapters device_adapters_network_profile_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_adapters
    ADD CONSTRAINT device_adapters_network_profile_project_fk FOREIGN KEY (network_profile_id, project_id) REFERENCES public.device_network_profiles(id, project_id) ON DELETE SET NULL (network_profile_id);


--
-- Name: device_adapters device_adapters_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_adapters
    ADD CONSTRAINT device_adapters_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_capabilities device_capabilities_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capabilities
    ADD CONSTRAINT device_capabilities_adapter_project_fk FOREIGN KEY (declared_by_adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE SET NULL (declared_by_adapter_id);


--
-- Name: device_capabilities device_capabilities_device_id_devices_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capabilities
    ADD CONSTRAINT device_capabilities_device_id_devices_id_fk FOREIGN KEY (device_id) REFERENCES public.devices(id) ON DELETE CASCADE;


--
-- Name: device_capabilities device_capabilities_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capabilities
    ADD CONSTRAINT device_capabilities_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: device_capabilities device_capabilities_device_type_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capabilities
    ADD CONSTRAINT device_capabilities_device_type_fk FOREIGN KEY (device_type_id) REFERENCES public.device_types(id) ON DELETE RESTRICT;


--
-- Name: device_capabilities device_capabilities_driver_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capabilities
    ADD CONSTRAINT device_capabilities_driver_fk FOREIGN KEY (driver_definition_id) REFERENCES public.driver_definitions(id) ON DELETE RESTRICT;


--
-- Name: device_capability_grants device_capability_grants_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capability_grants
    ADD CONSTRAINT device_capability_grants_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: device_capability_grants device_capability_grants_granter_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capability_grants
    ADD CONSTRAINT device_capability_grants_granter_fk FOREIGN KEY (granted_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: device_capability_grants device_capability_grants_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capability_grants
    ADD CONSTRAINT device_capability_grants_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_capability_grants device_capability_grants_team_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capability_grants
    ADD CONSTRAINT device_capability_grants_team_member_fk FOREIGN KEY (team_id, user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE CASCADE;


--
-- Name: device_capability_grants device_capability_grants_type_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_capability_grants
    ADD CONSTRAINT device_capability_grants_type_fk FOREIGN KEY (device_type_id) REFERENCES public.device_types(id) ON DELETE CASCADE;


--
-- Name: device_command_protocol_correlations device_command_protocol_correlations_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_command_protocol_correlations
    ADD CONSTRAINT device_command_protocol_correlations_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: device_command_protocol_correlations device_command_protocol_correlations_command_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_command_protocol_correlations
    ADD CONSTRAINT device_command_protocol_correlations_command_project_fk FOREIGN KEY (command_id, project_id) REFERENCES public.device_commands(id, project_id) ON DELETE CASCADE;


--
-- Name: device_command_protocol_correlations device_command_protocol_correlations_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_command_protocol_correlations
    ADD CONSTRAINT device_command_protocol_correlations_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_commands device_commands_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_commands
    ADD CONSTRAINT device_commands_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE RESTRICT;


--
-- Name: device_commands device_commands_live_stream_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_commands
    ADD CONSTRAINT device_commands_live_stream_project_fk FOREIGN KEY (live_stream_id, project_id) REFERENCES public.live_streams(id, project_id) ON DELETE SET NULL (live_stream_id);


--
-- Name: device_commands device_commands_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_commands
    ADD CONSTRAINT device_commands_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_commands device_commands_requester_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_commands
    ADD CONSTRAINT device_commands_requester_fk FOREIGN KEY (requested_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: device_commands device_commands_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_commands
    ADD CONSTRAINT device_commands_run_project_fk FOREIGN KEY (task_run_id, project_id) REFERENCES public.task_runs(id, project_id) ON DELETE CASCADE;


--
-- Name: device_commands device_commands_run_step_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_commands
    ADD CONSTRAINT device_commands_run_step_project_fk FOREIGN KEY (task_run_step_id, project_id) REFERENCES public.task_run_steps(id, project_id) ON DELETE CASCADE;


--
-- Name: device_connections device_connections_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connections
    ADD CONSTRAINT device_connections_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: device_connections device_connections_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connections
    ADD CONSTRAINT device_connections_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: device_connections device_connections_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connections
    ADD CONSTRAINT device_connections_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_connector_bindings device_connector_bindings_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connector_bindings
    ADD CONSTRAINT device_connector_bindings_connector_project_fk FOREIGN KEY (connector_instance_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: device_connector_bindings device_connector_bindings_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connector_bindings
    ADD CONSTRAINT device_connector_bindings_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: device_connector_bindings device_connector_bindings_identity_connector_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connector_bindings
    ADD CONSTRAINT device_connector_bindings_identity_connector_project_fk FOREIGN KEY (external_identity_id, connector_instance_id, project_id) REFERENCES public.device_external_identities(id, adapter_id, project_id) ON DELETE CASCADE;


--
-- Name: device_connector_bindings device_connector_bindings_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_connector_bindings
    ADD CONSTRAINT device_connector_bindings_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_external_identities device_external_identities_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_external_identities
    ADD CONSTRAINT device_external_identities_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: device_external_identities device_external_identities_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_external_identities
    ADD CONSTRAINT device_external_identities_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: device_external_identities device_external_identities_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_external_identities
    ADD CONSTRAINT device_external_identities_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_external_identities device_external_identities_suggested_type_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_external_identities
    ADD CONSTRAINT device_external_identities_suggested_type_fk FOREIGN KEY (suggested_device_type_id) REFERENCES public.device_types(id) ON DELETE SET NULL;


--
-- Name: device_external_identities device_external_identities_sync_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_external_identities
    ADD CONSTRAINT device_external_identities_sync_run_project_fk FOREIGN KEY (last_sync_run_id, project_id) REFERENCES public.connector_sync_runs(id, project_id) ON DELETE SET NULL (last_sync_run_id);


--
-- Name: device_latest_telemetry device_latest_telemetry_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_latest_telemetry
    ADD CONSTRAINT device_latest_telemetry_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: device_latest_telemetry device_latest_telemetry_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_latest_telemetry
    ADD CONSTRAINT device_latest_telemetry_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: device_network_profiles device_network_profiles_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_network_profiles
    ADD CONSTRAINT device_network_profiles_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_protocol_cursors device_protocol_cursors_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_protocol_cursors
    ADD CONSTRAINT device_protocol_cursors_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: device_protocol_cursors device_protocol_cursors_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_protocol_cursors
    ADD CONSTRAINT device_protocol_cursors_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_protocol_messages device_protocol_messages_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_protocol_messages
    ADD CONSTRAINT device_protocol_messages_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: device_protocol_messages device_protocol_messages_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_protocol_messages
    ADD CONSTRAINT device_protocol_messages_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_relationships device_relationships_from_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_relationships
    ADD CONSTRAINT device_relationships_from_project_fk FOREIGN KEY (from_device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: device_relationships device_relationships_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_relationships
    ADD CONSTRAINT device_relationships_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_relationships device_relationships_to_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_relationships
    ADD CONSTRAINT device_relationships_to_project_fk FOREIGN KEY (to_device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: device_stream_channels device_stream_channels_capability_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_stream_channels
    ADD CONSTRAINT device_stream_channels_capability_fk FOREIGN KEY (device_id, capability_code) REFERENCES public.device_capabilities(device_id, capability_code) ON DELETE CASCADE;


--
-- Name: device_stream_channels device_stream_channels_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_stream_channels
    ADD CONSTRAINT device_stream_channels_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: device_stream_channels device_stream_channels_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_stream_channels
    ADD CONSTRAINT device_stream_channels_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_telemetry device_telemetry_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE public.device_telemetry
    ADD CONSTRAINT device_telemetry_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: device_telemetry device_telemetry_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE public.device_telemetry
    ADD CONSTRAINT device_telemetry_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: device_telemetry device_telemetry_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE public.device_telemetry
    ADD CONSTRAINT device_telemetry_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: device_types device_types_driver_definition_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_types
    ADD CONSTRAINT device_types_driver_definition_id_fkey FOREIGN KEY (driver_definition_id) REFERENCES public.driver_definitions(id) ON DELETE RESTRICT;


--
-- Name: devices devices_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE SET NULL (adapter_id);


--
-- Name: devices devices_device_type_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_device_type_fk FOREIGN KEY (device_type_id) REFERENCES public.device_types(id) ON DELETE RESTRICT;


--
-- Name: devices devices_project_id_projects_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_project_id_projects_id_fk FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: devices devices_responsible_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_responsible_user_fk FOREIGN KEY (responsible_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: event_feedback event_feedback_actor_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_feedback
    ADD CONSTRAINT event_feedback_actor_fk FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: event_feedback event_feedback_event_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_feedback
    ADD CONSTRAINT event_feedback_event_project_fk FOREIGN KEY (perception_event_id, project_id) REFERENCES public.perception_events(id, project_id) ON DELETE CASCADE;


--
-- Name: event_feedback event_feedback_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_feedback
    ADD CONSTRAINT event_feedback_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: event_rule_versions event_rule_versions_creator_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rule_versions
    ADD CONSTRAINT event_rule_versions_creator_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: event_rule_versions event_rule_versions_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rule_versions
    ADD CONSTRAINT event_rule_versions_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: event_rule_versions event_rule_versions_publisher_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rule_versions
    ADD CONSTRAINT event_rule_versions_publisher_fk FOREIGN KEY (published_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: event_rule_versions event_rule_versions_rule_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rule_versions
    ADD CONSTRAINT event_rule_versions_rule_project_fk FOREIGN KEY (event_rule_id, project_id) REFERENCES public.event_rules(id, project_id) ON DELETE CASCADE;


--
-- Name: event_rules event_rules_creator_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rules
    ADD CONSTRAINT event_rules_creator_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: event_rules event_rules_current_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rules
    ADD CONSTRAINT event_rules_current_version_project_fk FOREIGN KEY (current_published_version_id, project_id) REFERENCES public.event_rule_versions(id, project_id) ON DELETE SET NULL (current_published_version_id);


--
-- Name: event_rules event_rules_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_rules
    ADD CONSTRAINT event_rules_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: evidence_links evidence_links_asset_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evidence_links
    ADD CONSTRAINT evidence_links_asset_project_fk FOREIGN KEY (asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE RESTRICT;


--
-- Name: evidence_links evidence_links_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evidence_links
    ADD CONSTRAINT evidence_links_created_by_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: evidence_links evidence_links_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evidence_links
    ADD CONSTRAINT evidence_links_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: generated_report_evidence generated_report_evidence_asset_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_report_evidence
    ADD CONSTRAINT generated_report_evidence_asset_project_fk FOREIGN KEY (asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE RESTRICT;


--
-- Name: generated_report_evidence generated_report_evidence_report_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_report_evidence
    ADD CONSTRAINT generated_report_evidence_report_project_fk FOREIGN KEY (report_version_id, project_id) REFERENCES public.generated_report_versions(id, project_id) ON DELETE CASCADE;


--
-- Name: generated_report_versions generated_report_versions_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_report_versions
    ADD CONSTRAINT generated_report_versions_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: generated_report_versions generated_report_versions_report_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_report_versions
    ADD CONSTRAINT generated_report_versions_report_project_fk FOREIGN KEY (generated_report_id, project_id) REFERENCES public.generated_reports(id, project_id) ON DELETE CASCADE;


--
-- Name: generated_reports generated_reports_current_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_reports
    ADD CONSTRAINT generated_reports_current_version_project_fk FOREIGN KEY (current_published_version_id, project_id) REFERENCES public.generated_report_versions(id, project_id) ON DELETE SET NULL;


--
-- Name: generated_reports generated_reports_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_reports
    ADD CONSTRAINT generated_reports_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: idempotency_records idempotency_records_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.idempotency_records
    ADD CONSTRAINT idempotency_records_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: issue_assignees issue_assignees_actor_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_assignees
    ADD CONSTRAINT issue_assignees_actor_fk FOREIGN KEY (assigned_by_user_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: issue_assignees issue_assignees_agent_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_assignees
    ADD CONSTRAINT issue_assignees_agent_project_fk FOREIGN KEY (agent_id, project_id) REFERENCES public.agents(id, project_id) ON DELETE CASCADE;


--
-- Name: issue_assignees issue_assignees_issue_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_assignees
    ADD CONSTRAINT issue_assignees_issue_project_fk FOREIGN KEY (issue_id, project_id) REFERENCES public.issues(id, project_id) ON DELETE CASCADE;


--
-- Name: issue_assignees issue_assignees_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_assignees
    ADD CONSTRAINT issue_assignees_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: issue_assignees issue_assignees_user_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_assignees
    ADD CONSTRAINT issue_assignees_user_team_fk FOREIGN KEY (team_id, user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE CASCADE;


--
-- Name: issue_events issue_events_actor_agent_id_agents_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_events
    ADD CONSTRAINT issue_events_actor_agent_id_agents_id_fk FOREIGN KEY (actor_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;


--
-- Name: issue_events issue_events_actor_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_events
    ADD CONSTRAINT issue_events_actor_user_id_users_id_fk FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: issue_events issue_events_issue_id_issues_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_events
    ADD CONSTRAINT issue_events_issue_id_issues_id_fk FOREIGN KEY (issue_id) REFERENCES public.issues(id) ON DELETE CASCADE;


--
-- Name: issue_events issue_events_issue_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_events
    ADD CONSTRAINT issue_events_issue_project_fk FOREIGN KEY (issue_id, project_id) REFERENCES public.issues(id, project_id) ON DELETE CASCADE;


--
-- Name: issue_events issue_events_project_id_projects_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_events
    ADD CONSTRAINT issue_events_project_id_projects_id_fk FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: issue_feedback issue_feedback_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback
    ADD CONSTRAINT issue_feedback_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: issue_feedback issue_feedback_algorithm_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback
    ADD CONSTRAINT issue_feedback_algorithm_version_project_fk FOREIGN KEY (algorithm_definition_version_id, project_id) REFERENCES public.algorithm_definition_versions(id, project_id) ON DELETE RESTRICT;


--
-- Name: issue_feedback issue_feedback_detection_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback
    ADD CONSTRAINT issue_feedback_detection_project_fk FOREIGN KEY (detection_id, project_id) REFERENCES public.detections(id, project_id) ON DELETE RESTRICT;


--
-- Name: issue_feedback issue_feedback_issue_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback
    ADD CONSTRAINT issue_feedback_issue_project_fk FOREIGN KEY (issue_id, project_id) REFERENCES public.issues(id, project_id) ON DELETE CASCADE;


--
-- Name: issue_feedback issue_feedback_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback
    ADD CONSTRAINT issue_feedback_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: issue_feedback issue_feedback_task_step_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback
    ADD CONSTRAINT issue_feedback_task_step_project_fk FOREIGN KEY (task_run_step_id, project_id) REFERENCES public.task_run_steps(id, project_id) ON DELETE RESTRICT;


--
-- Name: issue_feedback issue_feedback_task_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_feedback
    ADD CONSTRAINT issue_feedback_task_version_project_fk FOREIGN KEY (task_version_id, project_id) REFERENCES public.task_versions(id, project_id) ON DELETE RESTRICT;


--
-- Name: issue_links issue_links_created_by_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_links
    ADD CONSTRAINT issue_links_created_by_user_id_users_id_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: issue_links issue_links_issue_id_issues_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_links
    ADD CONSTRAINT issue_links_issue_id_issues_id_fk FOREIGN KEY (issue_id) REFERENCES public.issues(id) ON DELETE CASCADE;


--
-- Name: issue_links issue_links_issue_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_links
    ADD CONSTRAINT issue_links_issue_project_fk FOREIGN KEY (issue_id, project_id) REFERENCES public.issues(id, project_id) ON DELETE CASCADE;


--
-- Name: issue_links issue_links_project_id_projects_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issue_links
    ADD CONSTRAINT issue_links_project_id_projects_id_fk FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: issues issues_assignee_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issues
    ADD CONSTRAINT issues_assignee_user_id_users_id_fk FOREIGN KEY (assignee_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: issues issues_opened_by_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issues
    ADD CONSTRAINT issues_opened_by_user_id_users_id_fk FOREIGN KEY (opened_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: issues issues_project_id_projects_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issues
    ADD CONSTRAINT issues_project_id_projects_id_fk FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: issues issues_task_run_id_task_runs_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issues
    ADD CONSTRAINT issues_task_run_id_task_runs_id_fk FOREIGN KEY (task_run_id) REFERENCES public.task_runs(id) ON DELETE SET NULL;


--
-- Name: issues issues_task_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issues
    ADD CONSTRAINT issues_task_run_project_fk FOREIGN KEY (task_run_id, project_id) REFERENCES public.task_runs(id, project_id) ON DELETE SET NULL (task_run_id);


--
-- Name: issues issues_task_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.issues
    ADD CONSTRAINT issues_task_version_project_fk FOREIGN KEY (task_version_id, project_id) REFERENCES public.task_versions(id, project_id) ON DELETE RESTRICT;


--
-- Name: live_streams live_streams_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.live_streams
    ADD CONSTRAINT live_streams_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE SET NULL (adapter_id);


--
-- Name: live_streams live_streams_channel_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.live_streams
    ADD CONSTRAINT live_streams_channel_project_fk FOREIGN KEY (stream_channel_id, project_id) REFERENCES public.device_stream_channels(id, project_id) ON DELETE SET NULL (stream_channel_id);


--
-- Name: live_streams live_streams_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.live_streams
    ADD CONSTRAINT live_streams_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: live_streams live_streams_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.live_streams
    ADD CONSTRAINT live_streams_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: live_streams live_streams_started_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.live_streams
    ADD CONSTRAINT live_streams_started_by_fk FOREIGN KEY (started_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: live_streams live_streams_task_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.live_streams
    ADD CONSTRAINT live_streams_task_run_project_fk FOREIGN KEY (task_run_id, project_id) REFERENCES public.task_runs(id, project_id) ON DELETE SET NULL (task_run_id);


--
-- Name: observations observations_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.observations
    ADD CONSTRAINT observations_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- Name: observations observations_calibration_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.observations
    ADD CONSTRAINT observations_calibration_project_fk FOREIGN KEY (calibration_id, project_id) REFERENCES public.sensor_calibrations(id, project_id) ON DELETE SET NULL (calibration_id);


--
-- Name: observations observations_crs_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.observations
    ADD CONSTRAINT observations_crs_project_fk FOREIGN KEY (original_crs_id, project_id) REFERENCES public.coordinate_references(id, project_id) ON DELETE SET NULL (original_crs_id);


--
-- Name: observations observations_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.observations
    ADD CONSTRAINT observations_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: observations observations_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.observations
    ADD CONSTRAINT observations_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: observations observations_task_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.observations
    ADD CONSTRAINT observations_task_run_project_fk FOREIGN KEY (task_run_id, project_id) REFERENCES public.task_runs(id, project_id) ON DELETE SET NULL (task_run_id);


--
-- Name: outbox_consumptions outbox_consumptions_event_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_consumptions
    ADD CONSTRAINT outbox_consumptions_event_id_fkey FOREIGN KEY (event_id) REFERENCES public.outbox_events(event_id) ON DELETE CASCADE;


--
-- Name: outbox_events outbox_events_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_events
    ADD CONSTRAINT outbox_events_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: perception_events perception_events_assignee_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.perception_events
    ADD CONSTRAINT perception_events_assignee_fk FOREIGN KEY (assigned_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: perception_events perception_events_group_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.perception_events
    ADD CONSTRAINT perception_events_group_project_fk FOREIGN KEY (detection_group_id, project_id) REFERENCES public.detection_groups(id, project_id) ON DELETE RESTRICT;


--
-- Name: perception_events perception_events_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.perception_events
    ADD CONSTRAINT perception_events_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: perception_events perception_events_rule_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.perception_events
    ADD CONSTRAINT perception_events_rule_version_project_fk FOREIGN KEY (event_rule_version_id, project_id) REFERENCES public.event_rule_versions(id, project_id) ON DELETE RESTRICT;


--
-- Name: platform_audit_events platform_audit_events_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_audit_events
    ADD CONSTRAINT platform_audit_events_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: poses poses_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.poses
    ADD CONSTRAINT poses_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: poses poses_observation_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.poses
    ADD CONSTRAINT poses_observation_project_fk FOREIGN KEY (observation_id, project_id) REFERENCES public.observations(id, project_id) ON DELETE CASCADE;


--
-- Name: project_events project_events_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_events
    ADD CONSTRAINT project_events_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: project_feature_flags project_feature_flags_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_feature_flags
    ADD CONSTRAINT project_feature_flags_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: project_feature_flags project_feature_flags_updated_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_feature_flags
    ADD CONSTRAINT project_feature_flags_updated_by_user_id_fkey FOREIGN KEY (updated_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: project_permissions project_permissions_granted_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_permissions
    ADD CONSTRAINT project_permissions_granted_by_user_id_fkey FOREIGN KEY (granted_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: project_permissions project_permissions_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_permissions
    ADD CONSTRAINT project_permissions_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: project_permissions project_permissions_team_member_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_permissions
    ADD CONSTRAINT project_permissions_team_member_fk FOREIGN KEY (team_id, user_id) REFERENCES public.team_members(team_id, user_id) ON DELETE CASCADE;


--
-- Name: projects projects_created_by_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_created_by_user_id_users_id_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: projects projects_current_safety_policy_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_current_safety_policy_project_fk FOREIGN KEY (current_safety_policy_version_id, id) REFERENCES public.safety_policy_versions(id, project_id) ON DELETE SET NULL (current_safety_policy_version_id);


--
-- Name: projects projects_team_id_teams_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_team_id_teams_id_fk FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: retention_cleanup_runs retention_cleanup_runs_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_cleanup_runs
    ADD CONSTRAINT retention_cleanup_runs_created_by_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: retention_cleanup_runs retention_cleanup_runs_policy_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_cleanup_runs
    ADD CONSTRAINT retention_cleanup_runs_policy_project_fk FOREIGN KEY (retention_policy_id, project_id) REFERENCES public.retention_policies(id, project_id) ON DELETE RESTRICT;


--
-- Name: retention_cleanup_runs retention_cleanup_runs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_cleanup_runs
    ADD CONSTRAINT retention_cleanup_runs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: retention_holds retention_holds_asset_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_holds
    ADD CONSTRAINT retention_holds_asset_project_fk FOREIGN KEY (asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE CASCADE;


--
-- Name: retention_holds retention_holds_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_holds
    ADD CONSTRAINT retention_holds_created_by_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: retention_holds retention_holds_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_holds
    ADD CONSTRAINT retention_holds_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: retention_holds retention_holds_released_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_holds
    ADD CONSTRAINT retention_holds_released_by_fk FOREIGN KEY (released_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: retention_policies retention_policies_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_policies
    ADD CONSTRAINT retention_policies_created_by_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: retention_policies retention_policies_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_policies
    ADD CONSTRAINT retention_policies_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: retention_policies retention_policies_published_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_policies
    ADD CONSTRAINT retention_policies_published_by_fk FOREIGN KEY (published_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: retention_deletion_tombstones retention_tombstones_asset_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_deletion_tombstones
    ADD CONSTRAINT retention_tombstones_asset_project_fk FOREIGN KEY (asset_id, project_id) REFERENCES public.assets(id, project_id) ON DELETE RESTRICT;


--
-- Name: retention_deletion_tombstones retention_tombstones_policy_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_deletion_tombstones
    ADD CONSTRAINT retention_tombstones_policy_project_fk FOREIGN KEY (retention_policy_id, project_id) REFERENCES public.retention_policies(id, project_id) ON DELETE RESTRICT;


--
-- Name: retention_deletion_tombstones retention_tombstones_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_deletion_tombstones
    ADD CONSTRAINT retention_tombstones_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: retention_deletion_tombstones retention_tombstones_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.retention_deletion_tombstones
    ADD CONSTRAINT retention_tombstones_run_project_fk FOREIGN KEY (cleanup_run_id, project_id) REFERENCES public.retention_cleanup_runs(id, project_id) ON DELETE RESTRICT;


--
-- Name: safety_policy_versions safety_policy_versions_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.safety_policy_versions
    ADD CONSTRAINT safety_policy_versions_created_by_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: safety_policy_versions safety_policy_versions_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.safety_policy_versions
    ADD CONSTRAINT safety_policy_versions_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: safety_policy_versions safety_policy_versions_published_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.safety_policy_versions
    ADD CONSTRAINT safety_policy_versions_published_by_fk FOREIGN KEY (published_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: sensor_calibrations sensor_calibrations_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sensor_calibrations
    ADD CONSTRAINT sensor_calibrations_device_project_fk FOREIGN KEY (device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE CASCADE;


--
-- Name: sensor_calibrations sensor_calibrations_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sensor_calibrations
    ADD CONSTRAINT sensor_calibrations_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: task_run_steps task_run_steps_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_run_steps
    ADD CONSTRAINT task_run_steps_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: task_run_steps task_run_steps_run_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_run_steps
    ADD CONSTRAINT task_run_steps_run_project_fk FOREIGN KEY (task_run_id, project_id) REFERENCES public.task_runs(id, project_id) ON DELETE CASCADE;


--
-- Name: task_run_steps task_run_steps_step_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_run_steps
    ADD CONSTRAINT task_run_steps_step_project_fk FOREIGN KEY (task_step_id, project_id) REFERENCES public.task_steps(id, project_id) ON DELETE RESTRICT;


--
-- Name: task_runs task_runs_approval_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_approval_project_fk FOREIGN KEY (approval_request_id, project_id) REFERENCES public.approval_requests(id, project_id) ON DELETE RESTRICT;


--
-- Name: task_runs task_runs_created_by_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_created_by_user_id_users_id_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: task_runs task_runs_device_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_device_project_fk FOREIGN KEY (selected_device_id, project_id) REFERENCES public.devices(id, project_id) ON DELETE SET NULL (selected_device_id);


--
-- Name: task_runs task_runs_policy_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_policy_project_fk FOREIGN KEY (safety_policy_version_id, project_id) REFERENCES public.safety_policy_versions(id, project_id) ON DELETE RESTRICT;


--
-- Name: task_runs task_runs_project_id_projects_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_project_id_projects_id_fk FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: task_runs task_runs_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: task_runs task_runs_responsible_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_responsible_user_fk FOREIGN KEY (responsible_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: task_runs task_runs_takeoff_confirmer_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_takeoff_confirmer_fk FOREIGN KEY (takeoff_confirmed_by_user_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: task_runs task_runs_task_id_tasks_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_task_id_tasks_id_fk FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;


--
-- Name: task_runs task_runs_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_runs
    ADD CONSTRAINT task_runs_version_project_fk FOREIGN KEY (task_version_id, project_id) REFERENCES public.task_versions(id, project_id) ON DELETE RESTRICT;


--
-- Name: task_steps task_steps_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_steps
    ADD CONSTRAINT task_steps_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: task_steps task_steps_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_steps
    ADD CONSTRAINT task_steps_version_project_fk FOREIGN KEY (task_version_id, project_id) REFERENCES public.task_versions(id, project_id) ON DELETE CASCADE;


--
-- Name: task_versions task_versions_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_versions
    ADD CONSTRAINT task_versions_created_by_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: task_versions task_versions_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_versions
    ADD CONSTRAINT task_versions_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: task_versions task_versions_published_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_versions
    ADD CONSTRAINT task_versions_published_by_fk FOREIGN KEY (published_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: task_versions task_versions_task_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_versions
    ADD CONSTRAINT task_versions_task_project_fk FOREIGN KEY (task_id, project_id) REFERENCES public.tasks(id, project_id) ON DELETE CASCADE;


--
-- Name: tasks tasks_created_by_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_created_by_user_id_users_id_fk FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: tasks tasks_current_version_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_current_version_project_fk FOREIGN KEY (current_published_version_id, project_id) REFERENCES public.task_versions(id, project_id) ON DELETE SET NULL (current_published_version_id);


--
-- Name: tasks tasks_project_id_projects_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_project_id_projects_id_fk FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: tasks tasks_project_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_project_team_fk FOREIGN KEY (project_id, team_id) REFERENCES public.projects(id, team_id) ON DELETE CASCADE;


--
-- Name: team_members team_members_team_id_teams_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT team_members_team_id_teams_id_fk FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: team_members team_members_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT team_members_user_id_users_id_fk FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: telemetry_event_dedup telemetry_event_dedup_adapter_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_event_dedup
    ADD CONSTRAINT telemetry_event_dedup_adapter_project_fk FOREIGN KEY (adapter_id, project_id) REFERENCES public.device_adapters(id, project_id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--
