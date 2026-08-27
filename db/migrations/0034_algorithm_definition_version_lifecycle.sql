create or replace function protect_published_algorithm_definition_version()
returns trigger language plpgsql as $$
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
