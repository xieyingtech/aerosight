-- name: CreateAlgorithmDefinition :one
INSERT INTO algorithm_definitions(project_id,team_id,provider_id,name,capability_code,description,created_by_user_id)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id;

-- name: LockAlgorithmDefinition :one
SELECT id FROM algorithm_definitions WHERE project_id=$1 AND id=$2 FOR UPDATE;

-- name: NextAlgorithmConfigurationVersion :one
SELECT (coalesce(max(version),0)+1)::int FROM algorithm_definition_versions WHERE project_id=$1 AND algorithm_definition_id=$2;

-- name: UpdateAlgorithmDefinition :exec
UPDATE algorithm_definitions SET provider_id=$3,name=$4,capability_code=$5,description=$6,updated_at=now() WHERE project_id=$1 AND id=$2;

-- name: RetireAlgorithmConfigurations :exec
UPDATE algorithm_definition_versions SET status='retired' WHERE project_id=$1 AND algorithm_definition_id=$2 AND status='published';

-- name: InsertAlgorithmConfiguration :one
INSERT INTO algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,
 input_requirements_json,parameters_schema_json,protocol_config_json,output_mapping_json,label_mapping_json,output_schema_json,display_metadata_json,publish_threshold,created_by_user_id,published_by_user_id,published_at)
VALUES($1,$2,$3,$4,'published',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,sqlc.arg(actor_user_id),sqlc.arg(actor_user_id),now()) RETURNING id;

-- name: SetAlgorithmCurrentConfiguration :exec
UPDATE algorithm_definitions SET current_published_version_id=$3,updated_at=now() WHERE project_id=$1 AND id=$2;

-- name: ListAlgorithmCatalog :many
SELECT jsonb_build_object('id',definition.id::text,'configurationSnapshotId',version.id::text,'name',definition.name,'description',definition.description,
 'capabilityCode',definition.capability_code,'execution',jsonb_build_object('mode',version.execution_mode,'modelOrProcess',version.model_or_process),
 'provider',jsonb_build_object('type',provider.provider_type,'available',provider.status='active'),
 'schemas',jsonb_build_object('input',version.input_requirements_json,'parameters',version.parameters_schema_json,'output',version.output_schema_json),'display',version.display_metadata_json)
FROM algorithm_definitions definition
JOIN algorithm_definition_versions version ON version.id=definition.current_published_version_id AND version.project_id=definition.project_id
JOIN algorithm_providers provider ON provider.id=definition.provider_id
WHERE definition.project_id=$1 AND version.status='published' ORDER BY definition.name;
