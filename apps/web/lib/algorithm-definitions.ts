import "server-only";

import type { PoolClient } from "pg";

import { withAuditedProjectWrite } from "@/lib/audit";
import {
  algorithmDefinitionInputSchema,
  algorithmDefinitionVersionInputSchema,
  type AlgorithmDefinitionVersionInput
} from "@/lib/algorithm-definition-schema";
import { requireCurrentProjectPermission } from "@/lib/data";
import { correlationId } from "@/lib/observability";

async function managedWrite<T>(projectId: number, action: string, resourceId: string | undefined, input: unknown, requestId: string | null | undefined, write: (client: PoolClient, actorUserId: number) => Promise<T>) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "algorithm:manage");
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, actorUserId: user.id, requestId: correlationId(requestId),
    action, resourceType: "algorithm_definition", resourceId, input,
    policyResult: { permission: "algorithm:manage", immutableVersions: true }
  }, (client) => write(client, user.id));
}

export async function createAlgorithmDefinition(projectId: number, rawDefinition: unknown, rawVersion: unknown, requestId?: string | null) {
  const definition = algorithmDefinitionInputSchema.parse(rawDefinition);
  const version = algorithmDefinitionVersionInputSchema.parse(rawVersion);
  return managedWrite(projectId, "algorithm_definition.create", undefined, { definition, version }, requestId, async (client, actorUserId) => {
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
    const createdVersion = await insertVersion(client, projectId, provider.rows[0].teamId, Number(created.id), 1, version, actorUserId);
    return { definitionId: created.id, versionId: createdVersion.id, version: 1 };
  });
}

async function insertVersion(client: PoolClient, projectId: number, teamId: number, definitionId: number, versionNumber: number, version: AlgorithmDefinitionVersionInput, actorUserId: number) {
  return (await client.query<{ id: string }>(
    `insert into algorithm_definition_versions (
       project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,
       input_requirements_json,parameters_schema_json,protocol_config_json,output_mapping_json,label_mapping_json,
       output_schema_json,display_metadata_json,publish_threshold,created_by_user_id
     ) values ($1,$2,$3,$4,'draft',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) returning id`,
    [projectId, teamId, definitionId, versionNumber, version.executionMode, version.modelOrProcess,
      version.inputSchema, version.parametersSchema, version.protocolConfig, version.outputMapping, version.labelMapping,
      version.outputSchema, version.displayMetadata, version.publishThreshold, actorUserId]
  )).rows[0];
}

export async function createAlgorithmDefinitionVersion(projectId: number, definitionId: number, rawVersion: unknown, requestId?: string | null) {
  const version = algorithmDefinitionVersionInputSchema.parse(rawVersion);
  return managedWrite(projectId, "algorithm_definition.version.create", String(definitionId), version, requestId, async (client, actorUserId) => {
    const definition = (await client.query<{ teamId: number }>(
      `select team_id as "teamId" from algorithm_definitions where project_id=$1 and id=$2 for update`, [projectId, definitionId]
    )).rows[0];
    if (!definition) throw new Error("ALGORITHM_DEFINITION_NOT_FOUND");
    const next = (await client.query<{ version: number }>(
      `select coalesce(max(version),0)::int+1 as version from algorithm_definition_versions where project_id=$1 and algorithm_definition_id=$2`,
      [projectId, definitionId]
    )).rows[0].version;
    const created = await insertVersion(client, projectId, definition.teamId, definitionId, next, version, actorUserId);
    return { definitionId, versionId: created.id, version: next };
  });
}

export async function publishAlgorithmDefinitionVersion(projectId: number, definitionId: number, versionId: number, requestId?: string | null) {
  return managedWrite(projectId, "algorithm_definition.version.publish", String(definitionId), { versionId }, requestId, async (client, actorUserId) => {
    const target = (await client.query<{ id: string }>(
      `select id from algorithm_definition_versions
        where project_id=$1 and algorithm_definition_id=$2 and id=$3 and status='draft' for update`,
      [projectId, definitionId, versionId]
    )).rows[0];
    if (!target) throw new Error("ALGORITHM_DEFINITION_DRAFT_NOT_FOUND");
    await client.query(
      `update algorithm_definition_versions set status='retired'
        where project_id=$1 and algorithm_definition_id=$2 and status='published'`, [projectId, definitionId]
    );
    await client.query(
      `update algorithm_definition_versions set status='published',published_by_user_id=$3,published_at=now()
        where project_id=$1 and id=$2`, [projectId, versionId, actorUserId]
    );
    await client.query(
      `update algorithm_definitions set current_published_version_id=$3,updated_at=now()
        where project_id=$1 and id=$2`, [projectId, definitionId, versionId]
    );
    return { definitionId, versionId, status: "published" as const };
  });
}
