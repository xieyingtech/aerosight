import "server-only";

import type { PoolClient } from "pg";

import { auditHash } from "@/lib/audit-boundary";
import { db } from "@/lib/db";

export type IdempotencyContext = {
  projectId: number;
  teamId: number;
  actorKey: `user:${number}` | `agent:${number}` | `service:${string}`;
  operation: string;
  idempotencyKey: string;
  request: unknown;
};

type ExistingRecord = {
  requestHash: string;
  status: "processing" | "completed" | "failed";
  response: unknown;
  errorCode: string | null;
};

export async function withIdempotentProjectOperation<T>(
  context: IdempotencyContext,
  operation: (client: PoolClient) => Promise<T>
): Promise<{ value: T; replayed: boolean }> {
  const requestHash = auditHash(context.request);
  const client = await db.connect();
  try {
    await client.query("begin");
    const inserted = await client.query<{ id: string }>(
      `insert into idempotency_records (
         project_id, team_id, actor_key, operation, idempotency_key, request_hash
       ) values ($1, $2, $3, $4, $5, $6)
       on conflict (project_id, actor_key, operation, idempotency_key) do nothing
       returning id`,
      [
        context.projectId,
        context.teamId,
        context.actorKey,
        context.operation,
        context.idempotencyKey,
        requestHash
      ]
    );

    if (inserted.rowCount === 1) {
      const value = await operation(client);
      await client.query(
        `update idempotency_records
            set status = 'completed', response_json = $2, completed_at = now()
          where id = $1`,
        [inserted.rows[0].id, value]
      );
      await client.query("commit");
      return { value, replayed: false };
    }

    const existing = await client.query<ExistingRecord>(
      `select request_hash as "requestHash", status, response_json as response, error_code as "errorCode"
         from idempotency_records
        where project_id = $1 and actor_key = $2 and operation = $3 and idempotency_key = $4`,
      [context.projectId, context.actorKey, context.operation, context.idempotencyKey]
    );
    const record = existing.rows[0];
    if (!record || record.requestHash !== requestHash) {
      throw new Error("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST");
    }
    if (record.status === "processing") throw new Error("IDEMPOTENCY_OPERATION_IN_PROGRESS");
    if (record.status === "failed") throw new Error(record.errorCode ?? "IDEMPOTENCY_OPERATION_FAILED");
    await client.query("commit");
    return { value: record.response as T, replayed: true };
  } catch (error) {
    await client.query("rollback");
    throw error;
  } finally {
    client.release();
  }
}
