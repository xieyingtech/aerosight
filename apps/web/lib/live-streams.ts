import "server-only";

import { randomUUID } from "node:crypto";

import { correlationId } from "@/lib/observability";
import { withAuditedProjectWrite } from "@/lib/audit";
import { db, query } from "@/lib/db";
import { requireCurrentProjectPermission } from "@/lib/data";
import {
  assertStreamCanStart,
  assertLiveStreamProjectScope,
  normalizeAdapterLiveStatus,
  playbackAvailability,
  SimulatorPlaybackLocator,
  type LiveStreamSession,
  type LiveStreamStatus
} from "@/lib/live-stream-core";
import { publishProjectEvent } from "@/lib/project-events";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

type LiveStreamRow = {
  id: string;
  projectId: number;
  teamId: number;
  deviceId: number;
  streamKey: string;
  sourceType: string;
  status: LiveStreamStatus;
  playbackRef: string | null;
  playbackLocatorExpiresAt: Date | null;
  lastActiveAt: Date | null;
  statusReason: string | null;
};

const projection = `id, project_id as "projectId", team_id as "teamId", device_id as "deviceId",
  stream_key as "streamKey", source_type as "sourceType", status,
  playback_ref as "playbackRef", playback_locator_expires_at as "playbackLocatorExpiresAt",
  last_active_at as "lastActiveAt", status_reason as "statusReason"`;

function publicSession(row: LiveStreamRow): LiveStreamSession {
  return {
    id: Number(row.id), projectId: row.projectId, deviceId: row.deviceId,
    streamKey: row.streamKey, sourceType: row.sourceType, status: row.status,
    playbackRef: row.playbackRef, lastActiveAt: row.lastActiveAt?.toISOString() ?? null,
    statusReason: row.statusReason
  };
}

export async function startLiveStream(
  projectId: number,
  deviceId: number,
  input: { streamKey?: string; taskRunId?: number },
  requestId?: string | null
) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "mission:operate");
  const streamKey = input.streamKey?.trim() || "camera.main";
  if (!/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$/.test(streamKey)) throw new Error("INVALID_STREAM_KEY");

  return withAuditedProjectWrite(
    {
      projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
      action: "live_stream.start", resourceType: "live_stream", input: { deviceId, streamKey, taskRunId: input.taskRunId },
      policyResult: { permission: "mission:operate" }
    },
    async (client) => {
      const deviceResult = await client.query<{
        status: string; adapterId: string | null; adapterType: string | null; capabilities: string[];
      }>(
        `select device.status, device.adapter_id as "adapterId", adapter.adapter_type as "adapterType",
                coalesce((select array_agg(capability.capability_code)
                            from device_capabilities capability
                           where capability.device_id = device.id
                             and capability.project_id = device.project_id), '{}') as capabilities
           from devices device
           left join device_adapters adapter
             on adapter.id = device.adapter_id and adapter.project_id = device.project_id
          where device.project_id = $1 and device.id = $2
          for update of device`,
        [projectId, deviceId]
      );
      const device = deviceResult.rows[0];
      if (!device) throw new Error("DEVICE_NOT_FOUND");
      assertStreamCanStart({ deviceStatus: device.status, capabilities: device.capabilities, adapterType: device.adapterType });
      if (device.adapterType !== "simulator") throw new Error("LIVE_STREAM_ADAPTER_UNAVAILABLE");

      const existing = await client.query<LiveStreamRow>(
        `select ${projection} from live_streams
          where project_id = $1 and device_id = $2 and stream_key = $3
            and status in ('starting', 'live', 'degraded', 'stopping')
          limit 1`,
        [projectId, deviceId, streamKey]
      );
      if (existing.rows[0]) return { session: publicSession(existing.rows[0]), replayed: true };

      const started = await client.query<LiveStreamRow>(
        `insert into live_streams (
           project_id, team_id, device_id, task_run_id, adapter_id, stream_key,
           source_type, status, playback_ref, started_by_user_id, last_active_at
         ) values ($1, $2, $3, $4, $5, $6, 'simulator', 'live', $7, $8, now())
         returning ${projection}`,
        [
          projectId, access.teamId, deviceId, input.taskRunId ?? null, device.adapterId,
          streamKey, `simulator://devices/${deviceId}/${streamKey}`, user.id
        ]
      );
      const session = publicSession(started.rows[0]);
      await publishProjectEvent(client, {
        projectId, teamId: access.teamId, eventId: randomUUID(), eventType: "live_stream.started",
        payload: { streamId: session.id, deviceId, streamKey }, enqueue: false
      });
      return { session, replayed: false };
    }
  );
}

export async function stopLiveStream(projectId: number, streamId: number, requestId?: string | null) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "mission:operate");
  return withAuditedProjectWrite(
    {
      projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
      action: "live_stream.stop", resourceType: "live_stream", resourceId: String(streamId), input: {},
      policyResult: { permission: "mission:operate" }
    },
    async (client) => {
      const result = await client.query<LiveStreamRow>(
        `update live_streams
            set status = 'stopped', ended_at = now(), playback_ref = null,
                playback_locator_expires_at = null, updated_at = now()
          where project_id = $1 and id = $2 and status in ('starting', 'live', 'degraded', 'stopping', 'failed')
          returning ${projection}`,
        [projectId, streamId]
      );
      if (!result.rows[0]) {
        const existing = await client.query<LiveStreamRow>(
          `select ${projection} from live_streams where project_id = $1 and id = $2`, [projectId, streamId]
        );
        if (!existing.rows[0]) throw new Error("LIVE_STREAM_NOT_FOUND");
        return { session: publicSession(existing.rows[0]), replayed: true };
      }
      const session = publicSession(result.rows[0]);
      await publishProjectEvent(client, {
        projectId, teamId: access.teamId, eventId: randomUUID(), eventType: "live_stream.stopped",
        payload: { streamId, deviceId: session.deviceId }, enqueue: false
      });
      return { session, replayed: false };
    }
  );
}

export async function getLiveStreamPlayback(projectId: number, streamId: number) {
  await requireCurrentProjectPermission(projectId, "project:view");
  const result = await query<LiveStreamRow>(
    `select ${projection} from live_streams where project_id = $1 and id = $2`, [projectId, streamId]
  );
  const row = result.rows[0];
  if (!row) throw new Error("LIVE_STREAM_NOT_FOUND");
  const session = publicSession(row);
  assertLiveStreamProjectScope(session, projectId);
  const availability = playbackAvailability(session, new Date(), row.playbackLocatorExpiresAt);
  if (!availability.available) return { session, available: false, reason: availability.reason };
  if (session.sourceType !== "simulator" || !session.playbackRef) {
    return { session, available: false, reason: "playback-adapter-unavailable" };
  }
  const locator = new SimulatorPlaybackLocator(getWebRuntimeConfig().authSecret).issue({
    projectId, streamId, playbackRef: session.playbackRef, ttlSeconds: 60
  });
  await query(
    `update live_streams set playback_locator_expires_at = $3, updated_at = now()
      where project_id = $1 and id = $2`,
    [projectId, streamId, locator.expiresAt]
  );
  return { session, available: true, locator };
}

export async function syncLiveStreamStatus(input: {
  projectId: number;
  teamId: number;
  deviceId: number;
  streamKey: string;
  adapterStatus: string;
  reason?: string;
}) {
  const status = normalizeAdapterLiveStatus(input.adapterStatus);
  const client = await db.connect();
  try {
    await client.query("begin");
    const result = await client.query<LiveStreamRow>(
      `update live_streams
          set status = $4, status_reason = $5,
              last_active_at = case when $4 in ('live', 'degraded') then now() else last_active_at end,
              ended_at = case when $4 in ('failed', 'stopped') then now() else null end,
              playback_ref = case when $4 in ('failed', 'stopped') then null else playback_ref end,
              updated_at = now()
        where id = (
          select id from live_streams
           where project_id = $1 and device_id = $2 and stream_key = $3
             and status in ('starting', 'live', 'degraded', 'stopping')
           order by started_at desc limit 1
           for update
        )
        returning ${projection}`,
      [input.projectId, input.deviceId, input.streamKey, status, input.reason ?? null]
    );
    if (!result.rows[0]) throw new Error("LIVE_STREAM_NOT_FOUND");
    const session = publicSession(result.rows[0]);
    await publishProjectEvent(client, {
      projectId: input.projectId, teamId: input.teamId, eventId: randomUUID(),
      eventType: "live_stream.status_changed",
      payload: { streamId: session.id, deviceId: input.deviceId, status, reason: input.reason },
      enqueue: false
    });
    await client.query("commit");
    return session;
  } catch (error) {
    await client.query("rollback");
    throw error;
  } finally {
    client.release();
  }
}
