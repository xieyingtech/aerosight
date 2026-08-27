import "server-only";

import { withAuditedProjectWrite } from "@/lib/audit";
import { algorithmProviderInputSchema, publicAlgorithmProvider, type AlgorithmProviderInput } from "@/lib/algorithm-provider-policy";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";

type ProviderRow = {
  id: string; projectId: number; name: string; providerType: AlgorithmProviderInput["providerType"];
  baseUrl: string; secretRef: string | null; authType: AlgorithmProviderInput["authType"];
  allowedHeaders: string[]; timeoutSeconds: number; concurrencyLimit: number; rateLimitPerMinute: number;
  status: string; health: Record<string, unknown>; updatedAt: Date;
};

const projection = `id, project_id as "projectId", name, provider_type as "providerType", base_url as "baseUrl",
  secret_ref as "secretRef", auth_type as "authType", allowed_headers_json as "allowedHeaders",
  timeout_seconds as "timeoutSeconds", concurrency_limit as "concurrencyLimit",
  rate_limit_per_minute as "rateLimitPerMinute", status, health_json as health, updated_at as "updatedAt"`;

export async function listAlgorithmProviders(projectId: number) {
  await requireCurrentProjectPermission(projectId, "algorithm:manage");
  return (await query<ProviderRow>(`select ${projection} from algorithm_providers where project_id = $1 order by name`, [projectId])).rows.map(publicAlgorithmProvider);
}

export async function createAlgorithmProvider(projectId: number, rawInput: unknown, requestId?: string | null) {
  const input = algorithmProviderInputSchema.parse(rawInput);
  const { user, access } = await requireCurrentProjectPermission(projectId, "algorithm:manage");
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
    action: "algorithm_provider.create", resourceType: "algorithm_provider", input,
    policyResult: { permission: "algorithm:manage", secretReferenceOnly: true }
  }, async (client) => publicAlgorithmProvider((await client.query<ProviderRow>(
    `insert into algorithm_providers (
       project_id, team_id, name, provider_type, base_url, secret_ref, auth_type, allowed_headers_json,
       timeout_seconds, concurrency_limit, rate_limit_per_minute, created_by_user_id
     ) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) returning ${projection}`,
    [projectId, access.teamId, input.name, input.providerType, input.baseUrl, input.secretRef ?? null,
      input.authType, input.allowedHeaders, input.timeoutSeconds, input.concurrencyLimit, input.rateLimitPerMinute, user.id]
  )).rows[0]));
}

export async function updateAlgorithmProvider(projectId: number, providerId: number, rawInput: unknown, requestId?: string | null) {
  const input = algorithmProviderInputSchema.parse(rawInput);
  const { user, access } = await requireCurrentProjectPermission(projectId, "algorithm:manage");
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
    action: "algorithm_provider.update", resourceType: "algorithm_provider", resourceId: String(providerId), input,
    policyResult: { permission: "algorithm:manage", secretReferenceOnly: true }
  }, async (client) => {
    const result = await client.query<ProviderRow>(
      `update algorithm_providers set name=$3, provider_type=$4, base_url=$5, secret_ref=$6,
          auth_type=$7, allowed_headers_json=$8, timeout_seconds=$9, concurrency_limit=$10,
          rate_limit_per_minute=$11, updated_at=now() where project_id=$1 and id=$2 returning ${projection}`,
      [projectId, providerId, input.name, input.providerType, input.baseUrl, input.secretRef ?? null,
        input.authType, input.allowedHeaders, input.timeoutSeconds, input.concurrencyLimit, input.rateLimitPerMinute]
    );
    if (!result.rows[0]) throw new Error("ALGORITHM_PROVIDER_NOT_FOUND");
    return publicAlgorithmProvider(result.rows[0]);
  });
}
