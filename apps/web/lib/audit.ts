import "server-only";

import type { PoolClient } from "pg";

import { auditHash, executeWithinAuditBoundary } from "@/lib/audit-boundary";
import { db } from "@/lib/db";

export type AuditContext = {
  projectId: number;
  teamId: number;
  requestId: string;
  idempotencyKey?: string;
  actorUserId?: number;
  actorAgentId?: number;
  action: string;
  resourceType: string;
  resourceId?: string;
  input: unknown;
  policyResult?: Record<string, unknown>;
};

export async function withAuditedProjectWrite<T>(
  context: AuditContext,
  operation: (client: PoolClient) => Promise<T>
) {
  if (!context.actorUserId && !context.actorAgentId) {
    throw new Error("An audited write requires an actor");
  }
  const client = await db.connect();
  try {
    return await executeWithinAuditBoundary({
      begin: () => client.query("begin").then(() => undefined),
      writeAudit: async () => {
        const result = await client.query<{ id: string }>(
          `insert into audit_events (
             project_id, team_id, request_id, idempotency_key,
             actor_user_id, actor_agent_id, action, resource_type, resource_id,
             input_hash, policy_result_json
           ) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
           returning id`,
          [
            context.projectId,
            context.teamId,
            context.requestId,
            context.idempotencyKey ?? null,
            context.actorUserId ?? null,
            context.actorAgentId ?? null,
            context.action,
            context.resourceType,
            context.resourceId ?? null,
            auditHash(context.input),
            context.policyResult ?? {}
          ]
        );
        return Number(result.rows[0].id);
      },
      execute: () => operation(client),
      completeAudit: (auditId, result) => client.query(
        `update audit_events
            set status = 'completed', result_hash = $2, completed_at = now()
          where id = $1 and project_id = $3`,
        [auditId, auditHash(result), context.projectId]
      ).then(() => undefined),
      commit: () => client.query("commit").then(() => undefined),
      rollback: () => client.query("rollback").then(() => undefined)
    });
  } finally {
    client.release();
  }
}

export async function withAuditedPlatformWrite<T>(
  context: Omit<AuditContext, "projectId" | "teamId" | "actorAgentId" | "idempotencyKey" | "policyResult">,
  operation: (client: PoolClient) => Promise<T>
) {
  if (!context.actorUserId) throw new Error("An audited platform write requires an actor");
  const client = await db.connect();
  try {
    await client.query("begin");
    const audit = await client.query<{ id: string }>(
      `insert into platform_audit_events (
         actor_user_id, request_id, action, resource_type, resource_id, input_hash
       ) values ($1,$2,$3,$4,$5,$6) returning id`,
      [context.actorUserId, context.requestId, context.action, context.resourceType,
        context.resourceId ?? null, auditHash(context.input)]
    );
    const result = await operation(client);
    await client.query(
      `update platform_audit_events set status='completed', result_hash=$2, completed_at=now() where id=$1`,
      [audit.rows[0].id, auditHash(result)]
    );
    await client.query("commit");
    return result;
  } catch (error) {
    await client.query("rollback");
    throw error;
  } finally {
    client.release();
  }
}
