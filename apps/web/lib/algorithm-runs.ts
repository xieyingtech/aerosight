import "server-only";

import { randomUUID } from "node:crypto";
import { withAuditedProjectWrite } from "@/lib/audit";
import { buildAlgorithmRunDiagnostics, type AlgorithmRunViewRow } from "@/lib/algorithm-run-view-core";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";
import { publishProjectEvent } from "@/lib/project-events";

type AlgorithmRunRow = AlgorithmRunViewRow & {
  definitionName: string; definitionVersion: number; providerName: string; providerType: string;
  inputAssetId: number; taskRunId: number | null; deviceId: number | null; externalJobId: string | null;
};

const runProjection = `run.id, run.status, run.input_asset_id as "inputAssetId", run.task_run_id as "taskRunId",
  run.device_id as "deviceId", run.input_snapshot_json as "inputSnapshot", run.canonical_result_json as "canonicalResult",
  run.external_job_id as "externalJobId", run.raw_result_object_key as "rawResultObjectKey",
  run.raw_result_checksum_sha256 as "rawResultChecksumSha256", run.error_code as "errorCode",
  run.error_message as "errorMessage", run.created_at as "createdAt", run.started_at as "startedAt",
  run.finished_at as "finishedAt", definition.name as "definitionName", version.version as "definitionVersion",
  provider.name as "providerName", provider.provider_type as "providerType"`;

export async function listAlgorithmRuns(projectId: number) {
  await requireCurrentProjectPermission(projectId, "project:view");
  return (await query<AlgorithmRunRow>(`
    select ${runProjection} from algorithm_runs run
    join algorithm_definition_versions version on version.id=run.algorithm_definition_version_id and version.project_id=run.project_id
    join algorithm_definitions definition on definition.id=version.algorithm_definition_id and definition.project_id=run.project_id
    join algorithm_providers provider on provider.id=definition.provider_id and provider.project_id=run.project_id
    where run.project_id=$1 order by run.created_at desc limit 100`, [projectId])).rows;
}

export async function readAlgorithmRun(projectId: number, runId: string) {
  const { access } = await requireCurrentProjectPermission(projectId, "project:view");
  const run = (await query<AlgorithmRunRow>(`
    select ${runProjection} from algorithm_runs run
    join algorithm_definition_versions version on version.id=run.algorithm_definition_version_id and version.project_id=run.project_id
    join algorithm_definitions definition on definition.id=version.algorithm_definition_id and definition.project_id=run.project_id
    join algorithm_providers provider on provider.id=definition.provider_id and provider.project_id=run.project_id
    where run.project_id=$1 and run.id=$2`, [projectId, runId])).rows[0];
  if (!run) throw new Error("ALGORITHM_RUN_NOT_FOUND");
  const attempts = (await query<Record<string, unknown>>(`
    select attempt, status, response_status as "responseStatus", duration_ms as "durationMs",
           error_category as "errorCategory", external_job_id as "externalJobId",
           started_at as "startedAt", finished_at as "finishedAt"
      from algorithm_run_attempts where project_id=$1 and algorithm_run_id=$2 order by attempt`, [projectId, runId])).rows;
  return { run, attempts, view: buildAlgorithmRunDiagnostics(run, access.permissions) };
}

export async function retryAlgorithmRun(projectId: number, runId: string, requestId?: string | null) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "algorithm:manage");
  const retryRunId = randomUUID();
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, actorUserId: user.id, requestId: correlationId(requestId),
    action: "algorithm_run.retry", resourceType: "algorithm_run", resourceId: runId,
    input: { sourceRunId: runId }, policyResult: { permission: "algorithm:manage", terminalFailureOnly: true }
  }, async (client) => {
    const source = (await client.query<{
      teamId: number; versionId: string; inputAssetId: number; taskRunId: number | null; deviceId: number | null;
      parameters: Record<string, unknown>; inputSnapshot: Record<string, unknown>; status: string;
    }>(`select team_id as "teamId", algorithm_definition_version_id as "versionId", input_asset_id as "inputAssetId",
              task_run_id as "taskRunId", device_id as "deviceId", parameters_json as parameters,
              input_snapshot_json as "inputSnapshot", status
         from algorithm_runs where project_id=$1 and id=$2 for update`, [projectId, runId])).rows[0];
    if (!source) throw new Error("ALGORITHM_RUN_NOT_FOUND");
    if (!["failed", "timed_out"].includes(source.status)) throw new Error("ALGORITHM_RUN_NOT_RETRYABLE");
    const snapshot = structuredClone(source.inputSnapshot);
    delete snapshot.callback;
    await client.query(`insert into algorithm_runs (
      id, project_id, team_id, algorithm_definition_version_id, input_asset_id, task_run_id, device_id,
      idempotency_key, parameters_json, input_snapshot_json
    ) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, [retryRunId, projectId, source.teamId, source.versionId,
      source.inputAssetId, source.taskRunId, source.deviceId, `${runId}:retry:${retryRunId}`, source.parameters, snapshot]);
    await publishProjectEvent(client, { projectId, teamId: source.teamId, eventId: `algorithm-run-requested:${retryRunId}`,
      eventType: "algorithm.run.requested", payload: { runId: retryRunId } });
    return { runId: retryRunId };
  });
}
