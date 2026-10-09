-- Retire compatibility-only structures after checking production callers.
-- Keep complete historical rows in the existing audit archive, not public metadata.
-- Keep source-specific uniqueness explicit after compatibility cleanup.
drop index issue_links_issue_target_unique;
create unique index issue_links_issue_target_unique on issue_links(issue_id,link_type,target_id) where source_key is null;

-- 0090 archived retention rows. Carry active holds onto the existing asset flag.
update assets asset set legal_hold=true,
 retention_reason=coalesce(nullif(btrim(asset.retention_reason),''),archive.details_json->>'reason')
from audit_events archive
where archive.request_id='schema-0090' and archive.action='schema.archive'
 and archive.resource_type='retention_holds' and archive.details_json->>'status'='active'
 and asset.project_id=archive.project_id and asset.id=(archive.details_json->>'asset_id')::integer;

do $$
declare retired text;
begin
 foreach retired in array array['asset_upload_intents','evidence_links','event_feedback','asset_derivatives'] loop
  execute format($archive$
   insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
   select old.project_id,project.team_id,'schema-0091','schema-migration','schema.archive',%L,old.id::text,'schema-0091',to_jsonb(old),'completed',now()
   from %I old join projects project on project.id=old.project_id
  $archive$,retired,retired);
 end loop;
end;
$$;

-- The old download query preferred the upload filename over the display name.
-- Multiple historical intents for an asset are resolved deterministically.
update assets asset set metadata_json=asset.metadata_json || jsonb_build_object('uploadFileName',latest.file_name)
from (
 select distinct on(project_id,asset_id) project_id,asset_id,file_name
 from asset_upload_intents where asset_id is not null and file_name<>''
 order by project_id,asset_id,completed_at desc nulls last,created_at desc,id desc
) latest where asset.project_id=latest.project_id and asset.id=latest.asset_id;

update assets asset set metadata_json=asset.metadata_json || '{"legacyPublishedEvidence":true}'::jsonb
where exists(select 1 from evidence_links evidence where evidence.project_id=asset.project_id and evidence.asset_id=asset.id and evidence.is_published);

-- Preserve the old published-evidence delete restriction and sensitivity marker.
create function protect_legacy_published_asset() returns trigger language plpgsql as $$
begin
 if old.metadata_json->>'legacyPublishedEvidence'='true' then
  if tg_op='DELETE' then
   raise exception 'LEGACY_PUBLISHED_EVIDENCE_ASSET_IMMUTABLE' using errcode='55000';
  end if;
  if new.metadata_json->>'legacyPublishedEvidence' is distinct from 'true'
    or row(new.id,new.project_id) is distinct from row(old.id,old.project_id) then
   raise exception 'LEGACY_PUBLISHED_EVIDENCE_ASSET_IMMUTABLE' using errcode='55000';
  end if;
 end if;
 if tg_op='DELETE' then return old; end if;
 return new;
end;
$$;
create trigger assets_legacy_published_evidence before update or delete on assets
 for each row execute function protect_legacy_published_asset();

-- This API is read-only; snapshot its finite legacy history on the parent event.
alter table perception_events add column legacy_feedback_json jsonb not null default '[]'::jsonb
 check(jsonb_typeof(legacy_feedback_json)='array');
update perception_events event set legacy_feedback_json=history.feedback
from (
 select feedback.project_id,feedback.perception_event_id,
  jsonb_agg(to_jsonb(feedback) || jsonb_build_object('actor_name',actor.name) order by feedback.created_at,feedback.id) as feedback
 from event_feedback feedback join users actor on actor.id=feedback.actor_user_id
 group by feedback.project_id,feedback.perception_event_id
) history where event.project_id=history.project_id and event.id=history.perception_event_id;
comment on column perception_events.legacy_feedback_json is 'Read-only feedback from the retired event API; live feedback uses issue_feedback';

-- A generated thumbnail already is an asset. Its source belongs on that asset.
-- Legacy multi-source edges remain complete in audit_events; choose one primary
-- source deterministically for the current single-source generator contract.
alter table assets add column derivative_source_asset_id integer;
alter table assets add column derivative_type text;
alter table assets add constraint assets_derivative_source_project_fk
 foreign key(derivative_source_asset_id,project_id) references assets(id,project_id)
 on delete set null(derivative_source_asset_id);
alter table assets add constraint assets_derivative_not_self
 check(derivative_source_asset_id is null or (derivative_source_asset_id<>id and derivative_type is not null));
create index assets_derivative_source_idx on assets(project_id,derivative_source_asset_id)
 where derivative_source_asset_id is not null;
update assets asset set derivative_source_asset_id=source.source_asset_id,derivative_type=source.derivative_type,
 metadata_json=asset.metadata_json || jsonb_build_object('sourceAssetId',source.source_asset_id,
  'generator',source.generator,'generatorVersion',source.generator_version,'derivativeParameters',source.parameters_json)
from (
 select distinct on(project_id,derived_asset_id) * from asset_derivatives order by project_id,derived_asset_id,id
) source where asset.project_id=source.project_id and asset.id=source.derived_asset_id;

drop table asset_upload_intents;
drop table evidence_links;
drop table event_feedback;
drop table asset_derivatives;
drop function protect_published_evidence_link();
-- 0090 removed the associated tables, leaving these unused trigger functions.
drop function protect_published_retention_policy();
drop function protect_published_safety_policy_version();
