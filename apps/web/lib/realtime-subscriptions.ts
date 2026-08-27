import "server-only";

import type { PoolClient } from "pg";

import { db } from "@/lib/db";
import {
  authorizeRealtimeSubscription, encodeRealtimeAccessRevoked, encodeRealtimeSample, parseRealtimeCursor,
  realtimeFlowDecision,
  type RealtimeSample, type RealtimeSubscriptionGrant, type RealtimeSubscriptionTarget
} from "@/lib/realtime-subscription-core";

const encoder = new TextEncoder();
const replayLimit = 500;
const pollMilliseconds = 2_000;
const heartbeatMilliseconds = 15_000;
const permissionRecheckMilliseconds = 5_000;

export type ResolvedRealtimeSubscription = {
  stableChannelId: string;
  deviceId: number;
  dataType: "video" | "audio" | "telemetry" | "sensor" | "events";
  role: "owner" | "admin" | "member";
  target: RealtimeSubscriptionTarget;
};

export async function resolveRealtimeSubscription(
  client: PoolClient,
  userId: number,
  projectId: number,
  stableChannelId: string
) {
  const result = await client.query<ResolvedRealtimeSubscription>(
    `select channel.stable_channel_id as "stableChannelId",channel.device_id as "deviceId",
            channel.data_type as "dataType",membership.role,
            jsonb_build_object(
              'requestProjectId',$1::int,'channelProjectId',channel.project_id,
              'deviceId',channel.device_id,'deviceTypeId',device.device_type_id::text,
              'capabilityCode',channel.capability_code,'availability',channel.availability
            ) as target
       from device_stream_channels channel
       join devices device on device.id=channel.device_id and device.project_id=channel.project_id
       join projects project on project.id=channel.project_id
       join team_members membership on membership.team_id=project.team_id and membership.user_id=$2
      where channel.project_id=$1 and channel.stable_channel_id=$3`,
    [projectId, userId, stableChannelId]
  );
  const subscription = result.rows[0];
  if (!subscription) throw new Error("REALTIME_CHANNEL_NOT_FOUND");
  const grants = await client.query<RealtimeSubscriptionGrant>(
    `select scope_type as "scopeType",device_type_id::text as "deviceTypeId",device_id as "deviceId",
            action_pattern as "actionPattern",effect
       from device_capability_grants
      where project_id=$1 and user_id=$2 and (expires_at is null or expires_at>now())
        and (scope_type='project'
          or (scope_type='device_type' and device_type_id=$3::bigint)
          or (scope_type='device' and device_id=$4))`,
    [projectId, userId, subscription.target.deviceTypeId, subscription.deviceId]
  );
  authorizeRealtimeSubscription({ role: subscription.role, target: subscription.target, grants: grants.rows });
  return subscription;
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

async function readSamples(client: PoolClient, subscription: ResolvedRealtimeSubscription, projectId: number, cursor: string) {
  if (subscription.dataType === "telemetry" || subscription.dataType === "sensor") {
    return client.query<RealtimeSample>(
      `select id::text as cursor,event_id as "eventId",captured_at as "capturedAt",
              payload_json as payload,quality_json as quality
         from device_telemetry
        where project_id=$1 and device_id=$2 and id>$3::bigint
        order by id limit $4`,
      [projectId, subscription.deviceId, cursor, replayLimit + 1]
    );
  }
  return client.query<RealtimeSample>(
    `select cursor::text,event_id as "eventId",occurred_at as "capturedAt",
            payload_json as payload,'{}'::jsonb as quality
       from project_events
      where project_id=$1 and cursor>$3::bigint
        and payload_json->>'deviceId'=$2::text
      order by cursor limit $4`,
    [projectId, subscription.deviceId, cursor, replayLimit + 1]
  );
}

export function createRealtimeSubscriptionStream(input: {
  userId: number;
  projectId: number;
  stableChannelId: string;
  afterCursor: string | null;
  signal: AbortSignal;
}) {
  let cancelled = false;
  return new ReadableStream<Uint8Array>({
    async start(controller) {
      let cursor = parseRealtimeCursor(input.afterCursor);
      let lastHeartbeatAt = 0;
      let lastPermissionCheckAt = 0;
      let client: PoolClient | null = null;
      try {
        client = await db.connect();
        let subscription = await resolveRealtimeSubscription(client, input.userId, input.projectId, input.stableChannelId);
        while (!cancelled && !input.signal.aborted) {
          const now = Date.now();
          if (now - lastPermissionCheckAt >= permissionRecheckMilliseconds) {
            try {
              subscription = await resolveRealtimeSubscription(client, input.userId, input.projectId, input.stableChannelId);
            } catch {
              controller.enqueue(encoder.encode(encodeRealtimeAccessRevoked(input.stableChannelId)));
              break;
            }
            lastPermissionCheckAt = now;
          }
          const samples = await readSamples(client, subscription, input.projectId, cursor);
          if (realtimeFlowDecision(controller.desiredSize, samples.rows.length) === "terminate") {
            controller.enqueue(encoder.encode(`event: stream.closed\ndata: {"reason":"backpressure_limit_exceeded"}\n\n`));
            break;
          }
          for (const sample of samples.rows) {
            if (realtimeFlowDecision(controller.desiredSize, samples.rows.length) === "pause") break;
            controller.enqueue(encoder.encode(encodeRealtimeSample(input.stableChannelId, sample)));
            cursor = sample.cursor;
          }
          if (now - lastHeartbeatAt >= heartbeatMilliseconds) {
            controller.enqueue(encoder.encode(`: heartbeat ${new Date(now).toISOString()}\n\n`));
            lastHeartbeatAt = now;
          }
          await waitForPoll(input.signal);
        }
      } catch (error) {
        if (!cancelled && !input.signal.aborted) controller.error(error);
        return;
      } finally {
        client?.release();
      }
      controller.close();
    },
    cancel() {
      cancelled = true;
    }
  });
}
