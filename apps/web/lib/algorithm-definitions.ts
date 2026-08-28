import "server-only";

import type { PoolClient } from "pg";

import { withAuditedProjectWrite } from "@/lib/audit";
import {
  algorithmDefinitionConfigurationInputSchema,
  algorithmDefinitionInputSchema,
  type AlgorithmDefinitionConfigurationInput
} from "@/lib/algorithm-definition-schema";
import { requireCurrentProjectPermission } from "@/lib/data";
import { correlationId } from "@/lib/observability";

async function managedWrite<T>(projectId: number, action: string, resourceId: string | undefined, input: unknown, requestId: string | null | undefined, write: (client: PoolClient, actorUserId: number) => Promise<T>) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "algorithm:manage");
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, actorUserId: user.id, requestId: correlationId(requestId),
    action, resourceType: "algorithm_definition", resourceId, input,
    policyResult: { permission: "algorithm:manage", internalConfigurationSnapshots: true }
  }, (client) => write(client, user.id));
}

export async function createAlgorithmDefinition(projectId: number, rawDefinition: unknown, rawConfiguration: unknown, requestId?: string | null) {
  const definition = algorithmDefinitionInputSchema.parse(rawDefinition);
  const configuration = algorithmDefinitionConfigurationInputSchema.parse(rawConfiguration);
  return managedWrite(projectId, "algorithm_definition.save", undefined, { definition, configuration }, requestId, async (client, actorUserId) => {
    const provider = await client.query<{ teamId: number }>(
      `select team_id as "teamId" from algorithm_providers where project_id=$1 and id=$2 for share`,
      [projectId, definition.providerId]
    );
    if (!provider.rows[0]) throw new Error("ALGORITHM_PROVIDER_NOT_FOUND");
    const created = (await client.query<{ id: string }>(
      `insert into algorithm_definitions (project_id,team_id,provider_id,name,capability_code,description,created_by_user_id)
       values ($1,$2,$3,$4,$5,$6,$7) returning id`,
      [projectId, provider.rows[0].teamId, definition.providerId, definition.name, definition.capabilityCode, definition.description ?? null, actorUserId]
    )).rows[0];
    const snapshot = await insertConfigurationSnapshot(
      client, projectId, provider.rows[0].teamId, Number(created.id), 1, configuration, actorUserId
    );
    await setCurrentConfigurationSnapshot(client, projectId, Number(created.id), Number(snapshot.id));
    return { definitionId: created.id, configurationSnapshotId: snapshot.id };
  });
}

async function insertConfigurationSnapshot(client: PoolClient, projectId: number, teamId: number, definitionId: number, snapshotNumber: number, configuration: AlgorithmDefinitionConfigurationInput, actorUserId: number) {
  return (await client.query<{ id: string }>(
    `insert into algorithm_definition_versions (
       project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,
       input_requirements_json,parameters_schema_json,protocol_config_json,output_mapping_json,label_mapping_json,
       output_schema_json,display_metadata_json,publish_threshold,created_by_user_id,published_by_user_id,published_at
     ) values ($1,$2,$3,$4,'published',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$15,now()) returning id`,
    [projectId, teamId, definitionId, snapshotNumber, configuration.executionMode, configuration.modelOrProcess,
      configuration.inputSchema, configuration.parametersSchema, configuration.protocolConfig, configuration.outputMapping,
      configuration.labelMapping, configuration.outputSchema, configuration.displayMetadata,
      configuration.publishThreshold, actorUserId]
  )).rows[0];
}

async function retireCurrentConfigurationSnapshot(client: PoolClient, projectId: number, definitionId: number) {
  await client.query(
    `update algorithm_definition_versions set status='retired'
      where project_id=$1 and algorithm_definition_id=$2 and status='published'`,
    [projectId, definitionId]
  );
}

async function setCurrentConfigurationSnapshot(client: PoolClient, projectId: number, definitionId: number, snapshotId: number) {
  await client.query(
    `update algorithm_definitions set current_published_version_id=$3,updated_at=now()
      where project_id=$1 and id=$2`,
    [projectId, definitionId, snapshotId]
  );
}

export async function saveAlgorithmDefinition(
  projectId: number,
  definitionId: number,
  rawDefinition: unknown,
  rawConfiguration: unknown,
  requestId?: string | null
) {
  const definition = algorithmDefinitionInputSchema.parse(rawDefinition);
  const configuration = algorithmDefinitionConfigurationInputSchema.parse(rawConfiguration);
  return managedWrite(
    projectId,
    "algorithm_definition.save",
    String(definitionId),
    { definition, configuration },
    requestId,
    async (client, actorUserId) => {
      const provider = (await client.query<{ teamId: number }>(
        `select team_id as "teamId" from algorithm_providers where project_id=$1 and id=$2 for share`,
        [projectId, definition.providerId]
      )).rows[0];
      if (!provider) throw new Error("ALGORITHM_PROVIDER_NOT_FOUND");
      const current = (await client.query<{ id: string }>(
        `select id from algorithm_definitions where project_id=$1 and id=$2 for update`,
        [projectId, definitionId]
      )).rows[0];
      if (!current) throw new Error("ALGORITHM_DEFINITION_NOT_FOUND");
      const nextSnapshot = (await client.query<{ value: number }>(
        `select coalesce(max(version),0)::int+1 as value from algorithm_definition_versions
          where project_id=$1 and algorithm_definition_id=$2`,
        [projectId, definitionId]
      )).rows[0].value;
      await client.query(
        `update algorithm_definitions set provider_id=$3,name=$4,capability_code=$5,description=$6,updated_at=now()
          where project_id=$1 and id=$2`,
        [projectId, definitionId, definition.providerId, definition.name, definition.capabilityCode, definition.description ?? null]
      );
      await retireCurrentConfigurationSnapshot(client, projectId, definitionId);
      const snapshot = await insertConfigurationSnapshot(
        client, projectId, provider.teamId, definitionId, nextSnapshot, configuration, actorUserId
      );
      await setCurrentConfigurationSnapshot(client, projectId, definitionId, Number(snapshot.id));
      return { definitionId, configurationSnapshotId: snapshot.id };
    }
  );
}
