import "server-only";

import type { PoolClient } from "pg";
import { db } from "@/lib/db";
import {
  encodeServerSentEvent, encodeSnapshotRequired, parseEventCursor, replayDecision,
  shouldRecheckAuthorization, type StreamProjectEvent
} from "@/lib/project-stream-core";

const encoder = new TextEncoder();
const replayLimit = 500;
const pollMilliseconds = 2_000;
const heartbeatMilliseconds = 15_000;
const permissionRecheckMilliseconds = 15_000;

export async function canAccessProjectStream(client: PoolClient, userId: number, projectId: number) {
  const result = await client.query(
    `select 1 from projects project
     join team_members membership on membership.team_id = project.team_id
     where project.id = $1 and membership.user_id = $2`,
    [projectId, userId]
  );
  return Boolean(result.rowCount);
}

function waitForPoll(signal: AbortSignal) {
  return new Promise<void>((resolve) => {
    const timeout = setTimeout(resolve, pollMilliseconds);
    signal.addEventListener("abort", () => {
      clearTimeout(timeout);
      resolve();
    }, { once: true });
  });
}

export function createProjectEventStream(input: {
  userId: number;
  projectId: number;
  afterCursor: string | null;
  signal: AbortSignal;
}) {
  let cancelled = false;
  let activeClient: PoolClient | null = null;
  return new ReadableStream<Uint8Array>({
    async start(controller) {
      let cursor = parseEventCursor(input.afterCursor);
      let lastHeartbeatAt = 0;
      let lastPermissionCheckAt = 0;
      try {
        activeClient = await db.connect();
        while (!cancelled && !input.signal.aborted) {
          const now = Date.now();
          if (shouldRecheckAuthorization(lastPermissionCheckAt, now, permissionRecheckMilliseconds)) {
            if (!await canAccessProjectStream(activeClient, input.userId, input.projectId)) {
              controller.enqueue(encoder.encode("event: access.revoked\ndata: {\"reason\":\"project_membership_revoked\"}\n\n"));
              break;
            }
            lastPermissionCheckAt = now;
          }
          const events = await activeClient.query<StreamProjectEvent>(
            `select cursor::text, event_id as "eventId", event_type as "eventType",
                    payload_json as payload, occurred_at as "occurredAt"
             from project_events
             where project_id = $1 and cursor > $2::bigint
             order by cursor limit $3`,
            [input.projectId, cursor, replayLimit + 1]
          );
          const decision = replayDecision(events.rows, replayLimit);
          if (decision.kind === "snapshot-required") {
            controller.enqueue(encoder.encode(encodeSnapshotRequired(decision.cursor, input.projectId)));
            break;
          }
          for (const event of decision.events) {
            controller.enqueue(encoder.encode(encodeServerSentEvent(event)));
            cursor = event.cursor;
          }
          if (now - lastHeartbeatAt >= heartbeatMilliseconds) {
            controller.enqueue(encoder.encode(`: heartbeat ${new Date(now).toISOString()}\n\n`));
            lastHeartbeatAt = now;
          }
          await waitForPoll(input.signal);
        }
      } catch (error) {
        if (!input.signal.aborted && !cancelled) controller.error(error);
        return;
      } finally {
        activeClient?.release();
        activeClient = null;
      }
      controller.close();
    },
    cancel() {
      cancelled = true;
      activeClient?.release();
      activeClient = null;
    }
  });
}
