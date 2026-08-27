import "server-only";

import { randomUUID } from "node:crypto";

import { correlationId } from "@/lib/observability";
import { withAuditedProjectWrite } from "@/lib/audit";
import { db, query } from "@/lib/db";
import { requireCurrentProjectPermission } from "@/lib/data";
import {
  assertStreamCanStart,
  assertLiveStreamConcurrency,
  assertLiveStreamProjectScope,
  createLiveStreamIngestRef,
  normalizeAdapterLiveStatus,
  playbackAvailability,
  SimulatorPlaybackLocator,
  transitionLiveStream,
  type LiveStreamSession,
  type LiveStreamStatus
} from "@/lib/live-stream-core";
import { actionPatternMatches, authorizeCapabilityAction } from "@/lib/device-command-core";
import { publishProjectEvent } from "@/lib/project-events";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

type LiveStreamRow = {
  id: string;
  projectId: number;
  teamId: number;
  deviceId: number;
  streamKey: string;
  sourceType: string;
  streamChannelId: string | null;
  status: LiveStreamStatus;
  playbackRef: string | null;
  playbackLocatorExpiresAt: Date | null;
  lastActiveAt: Date | null;
  statusReason: string | null;
};

const projection = `id, project_id as "projectId", team_id as "teamId", device_id as "deviceId",
  stream_key as "streamKey", source_type as "sourceType", stream_channel_id as "streamChannelId", status,
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
  const requestedStreamKey = input.streamKey?.trim() || null;
  if (requestedStreamKey && !/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$/.test(requestedStreamKey)) {
    throw new Error("INVALID_STREAM_KEY");
  }

  return withAuditedProjectWrite(
    {
      projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
      action: "live_stream.start", resourceType: "live_stream", input: { deviceId, streamKey: requestedStreamKey, taskRunId: input.taskRunId },
      policyResult: { permission: "mission:operate" }
    },
    async (client) => {
      const deviceResult = await client.query<{
        status: string; adapterId: string | null; adapterType: string | null; capabilities: string[];
        deviceTypeId: string; maxConcurrentSessions: number;
      }>(
        `select device.status, device.adapter_id as "adapterId", adapter.adapter_type as "adapterType",
                device.device_type_id::text as "deviceTypeId",
                coalesce((select array_agg(capability.capability_code)
                            from device_capabilities capability
                           where capability.device_id = device.id
                             and capability.project_id = device.project_id
                             and capability.availability = 'available'), '{}') as capabilities,
                coalesce((select greatest(1, least(16,
                           coalesce((capability.constraints_json->>'maxConcurrentSessions')::int, 1)))
                            from device_capabilities capability
                           where capability.device_id=device.id and capability.project_id=device.project_id
                             and capability.capability_code in ('stream.video.control','camera.live')
                             and capability.availability='available'
                           order by capability.capability_code='stream.video.control' desc limit 1), 1)::int
                  as "maxConcurrentSessions"
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
      const controlAction = device.capabilities.includes("stream.video.control") ? "stream.video.control" : "camera.live";
      if (controlAction === "stream.video.control") {
        const grantRows = await client.query<{ actionPattern: string; effect: "allow" | "deny" }>(
          `select action_pattern as "actionPattern",effect from device_capability_grants
            where project_id=$1 and team_id=$2 and user_id=$3 and (expires_at is null or expires_at>now())
              and (scope_type='project' or (scope_type='device_type' and device_type_id=$4::bigint)
                   or (scope_type='device' and device_id=$5))`,
          [projectId, access.teamId, user.id, device.deviceTypeId, deviceId]
        );
        authorizeCapabilityAction({ role: access.role, action: controlAction,
          grants: grantRows.rows.filter((grant) => actionPatternMatches(grant.actionPattern, controlAction)) });
      }

      const channelResult = await client.query<{ id: string; channelKey: string }>(
        `select id::text,channel_key as "channelKey" from device_stream_channels
          where project_id=$1 and device_id=$2 and data_type='video' and availability='available'
            and ($3::text is null or channel_key=$3)
          order by channel_key limit 1`, [projectId, deviceId, requestedStreamKey]
      );
      const channel = channelResult.rows[0];
      if (!channel && device.adapterType !== "simulator") throw new Error("LIVE_STREAM_CHANNEL_NOT_FOUND");
      const streamKey = channel?.channelKey ?? requestedStreamKey ?? "camera.main";

      const existing = await client.query<LiveStreamRow>(
        `select ${projection} from live_streams
          where project_id = $1 and device_id = $2 and stream_key = $3
            and status in ('starting', 'live', 'degraded', 'stopping')
          limit 1`,
        [projectId, deviceId, streamKey]
      );
      if (existing.rows[0]) return { session: publicSession(existing.rows[0]), replayed: true };

      const active = await client.query<{ count: number }>(
        `select count(*)::int as count from live_streams
          where project_id=$1 and device_id=$2
            and status in ('requested','starting','live','degraded','stopping')`, [projectId, deviceId]
      );
      assertLiveStreamConcurrency({ activeCount: active.rows[0]?.count ?? 0,
        maxConcurrentSessions: device.maxConcurrentSessions });

      const sourceType = device.adapterType === "dji" ? "dji" : "simulator";
      const status: LiveStreamStatus = sourceType === "dji" ? "requested" : "live";
      const ingestRef = createLiveStreamIngestRef();
      const started = await client.query<LiveStreamRow>(
        `insert into live_streams (
           project_id, team_id, device_id, task_run_id, adapter_id, stream_key,
           stream_channel_id, source_type, status, ingest_ref, playback_ref, started_by_user_id,
           last_active_at, lease_owner, lease_expires_at
         ) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,
                   case when $9='live' then now() else null end,$13,now()+interval '45 seconds')
         returning ${projection}`,
        [
          projectId, access.teamId, deviceId, input.taskRunId ?? null, device.adapterId,
          streamKey, channel?.id ?? null, sourceType, status, ingestRef,
          sourceType === "simulator" ? `simulator://devices/${deviceId}/${streamKey}` : null,
          user.id, `web:${randomUUID()}`
        ]
      );
      const session = publicSession(started.rows[0]);
      await publishProjectEvent(client, {
        projectId, teamId: access.teamId, eventId: randomUUID(),
        eventType: status === "requested" ? "live_stream.requested" : "live_stream.started",
        payload: { streamId: session.id, deviceId, streamKey, status }, enqueue: false
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
            set status = case
                  when source_type='dji' and status in ('requested','starting','live','degraded') then 'stopping'
                  else 'stopped' end,
                ended_at = case
                  when source_type='dji' and status in ('requested','starting','live','degraded') then null
                  else now() end,
                playback_ref = case
                  when source_type='dji' and status in ('requested','starting','live','degraded') then playback_ref
                  else null end,
                playback_locator_expires_at = null,
                lease_owner = case
                  when source_type='dji' and status in ('requested','starting','live','degraded') then $3
                  else null end,
                lease_expires_at = case
                  when source_type='dji' and status in ('requested','starting','live','degraded') then now()+interval '45 seconds'
                  else null end,
                updated_at = now()
          where project_id = $1 and id = $2 and status in ('requested','starting','live','degraded','failed')
          returning ${projection}`,
        [projectId, streamId, `web:${randomUUID()}`]
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
        payload: { streamId, deviceId: session.deviceId, status: session.status }, enqueue: false
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
    const current = await client.query<LiveStreamRow>(
      `select ${projection} from live_streams
        where project_id=$1 and device_id=$2 and stream_key=$3
          and status in ('requested','starting','live','degraded','stopping')
        order by started_at desc limit 1 for update`,
      [input.projectId, input.deviceId, input.streamKey]
    );
    if (!current.rows[0]) throw new Error("LIVE_STREAM_NOT_FOUND");
    transitionLiveStream(current.rows[0].status, status);
    const result = await client.query<LiveStreamRow>(
      `update live_streams
          set status = $4, status_reason = $5,
              last_active_at = case when $4 in ('live', 'degraded') then now() else last_active_at end,
              ended_at = case when $4 in ('failed', 'stopped') then now() else null end,
              playback_ref = case when $4 in ('failed', 'stopped') then null else playback_ref end,
              lease_owner = case when $4 in ('failed','stopped') then null else lease_owner end,
              lease_expires_at = case
                when $4 in ('failed','stopped') then null else now()+interval '45 seconds' end,
              updated_at = now()
        where id=$6
        returning ${projection}`,
      [input.projectId, input.deviceId, input.streamKey, status, input.reason ?? null, current.rows[0].id]
    );
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

export async function recoverExpiredLiveStreams(limit = 100) {
  const boundedLimit = Math.min(500, Math.max(1, Math.trunc(limit)));
  const client = await db.connect();
  try {
    await client.query("begin");
    const recovered = await client.query<LiveStreamRow>(
      `with expired as (
         select id from live_streams
          where lease_expires_at<now()
            and status in ('requested','starting','live','degraded','stopping')
          order by lease_expires_at limit $1 for update skip locked
       )
       update live_streams stream set
         status=case when stream.status='stopping' then 'stopped' else 'failed' end,
         status_reason=case
           when stream.status in ('requested','starting') then 'session-start-lease-expired'
           when stream.status='stopping' then 'session-stop-lease-expired'
           else 'session-owner-lease-expired' end,
         ended_at=now(),playback_ref=null,playback_locator_expires_at=null,
         lease_owner=null,lease_expires_at=null,updated_at=now()
       from expired where stream.id=expired.id returning ${projection}`,
      [boundedLimit]
    );
    await client.query("commit");
    return recovered.rows.map(publicSession);
  } catch (error) {
    await client.query("rollback");
    throw error;
  } finally {
    client.release();
  }
}
