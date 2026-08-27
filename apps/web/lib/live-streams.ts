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
  buildDJIVideoID,
  buildPlaybackCandidates,
  buildRTMPIngestURL,
  MediaPlaybackTokenIssuer,
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
  vendorStreamRef: string | null;
  status: LiveStreamStatus;
  playbackRef: string | null;
  playbackLocatorExpiresAt: Date | null;
  lastActiveAt: Date | null;
  statusReason: string | null;
};

function mediaPublishCredentials(secretRef: string | null) {
  let referenced: Record<string, unknown> = {};
  if (secretRef && process.env[secretRef]) {
    try {
      referenced = JSON.parse(process.env[secretRef]!) as Record<string, unknown>;
    } catch {
      throw new Error("LIVE_STREAM_NETWORK_SECRET_INVALID");
    }
  }
  const username = typeof referenced.mediaPublishUser === "string"
    ? referenced.mediaPublishUser : process.env.MEDIA_PUBLISH_USER;
  const password = typeof referenced.mediaPublishPassword === "string"
    ? referenced.mediaPublishPassword : process.env.MEDIA_PUBLISH_PASSWORD;
  if (!username || !password) throw new Error("LIVE_STREAM_PUBLISH_CREDENTIALS_REQUIRED");
  return { username, password };
}

const projection = `id, project_id as "projectId", team_id as "teamId", device_id as "deviceId",
  stream_key as "streamKey", source_type as "sourceType", stream_channel_id as "streamChannelId",
  vendor_stream_ref as "vendorStreamRef", status,
  playback_ref as "playbackRef", playback_locator_expires_at as "playbackLocatorExpiresAt",
  last_active_at as "lastActiveAt", status_reason as "statusReason"`;

const qualifiedProjection = `stream.id,stream.project_id as "projectId",stream.team_id as "teamId",
  stream.device_id as "deviceId",stream.stream_key as "streamKey",stream.source_type as "sourceType",
  stream.stream_channel_id as "streamChannelId",stream.vendor_stream_ref as "vendorStreamRef",stream.status,
  stream.playback_ref as "playbackRef",stream.playback_locator_expires_at as "playbackLocatorExpiresAt",
  stream.last_active_at as "lastActiveAt",stream.status_reason as "statusReason"`;

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
        deviceTypeId: string; maxConcurrentSessions: number; mediaIngestBaseUrl: string | null;
        networkSecretRef: string | null;
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
                  as "maxConcurrentSessions",
                profile.media_ingest_base_url as "mediaIngestBaseUrl",
                profile.secret_ref as "networkSecretRef"
           from devices device
           left join device_adapters adapter
             on adapter.id = device.adapter_id and adapter.project_id = device.project_id
           left join device_network_profiles profile
             on profile.id=adapter.network_profile_id and profile.project_id=adapter.project_id
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
            and status in ('requested', 'starting', 'live', 'degraded', 'stopping')
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
      let vendorStreamRef: string | null = null;
      let vendorIngestURL: string | null = null;
      if (sourceType === "dji") {
        const topology = await client.query<{
          sourceExternalId: string; cameraType: number; cameraSubtype: number;
        }>(
          `select parent_identity.external_device_id as "sourceExternalId",
                  (camera_identity.identity_json->>'productType')::int as "cameraType",
                  (camera_identity.identity_json->>'productSubtype')::int as "cameraSubtype"
             from device_external_identities camera_identity
             join device_relationships relation
               on relation.project_id=camera_identity.project_id and relation.to_device_id=camera_identity.device_id
                 and relation.valid_until is null and relation.relation_type in ('contains','mounted-on')
             join device_external_identities parent_identity
               on parent_identity.project_id=relation.project_id and parent_identity.adapter_id=camera_identity.adapter_id
                 and parent_identity.device_id=relation.from_device_id
            where camera_identity.project_id=$1 and camera_identity.device_id=$2
            order by relation.valid_from desc limit 1`, [projectId, deviceId]
        );
        const source = topology.rows[0];
        if (!source) throw new Error("DJI_LIVE_TOPOLOGY_NOT_FOUND");
        vendorStreamRef = buildDJIVideoID(source);
        if (!device.mediaIngestBaseUrl) throw new Error("LIVE_STREAM_MEDIA_INGEST_PROFILE_REQUIRED");
        vendorIngestURL = buildRTMPIngestURL({ baseURL: device.mediaIngestBaseUrl, ingestRef,
          ...mediaPublishCredentials(device.networkSecretRef) });
      }
      const started = await client.query<LiveStreamRow>(
        `insert into live_streams (
           project_id, team_id, device_id, task_run_id, adapter_id, stream_key,
           stream_channel_id, source_type, status, ingest_ref, vendor_stream_ref, playback_ref, started_by_user_id,
           last_active_at, lease_owner, lease_expires_at
         ) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,
                   case when $9='live' then now() else null end,$14,now()+interval '45 seconds')
         returning ${projection}`,
        [
          projectId, access.teamId, deviceId, input.taskRunId ?? null, device.adapterId,
          streamKey, channel?.id ?? null, sourceType, status, ingestRef, vendorStreamRef,
          sourceType === "simulator" ? `simulator://devices/${deviceId}/${streamKey}` : null,
          user.id, `web:${randomUUID()}`
        ]
      );
      const session = publicSession(started.rows[0]);
      if (sourceType === "dji") {
        const commandId = randomUUID();
        await client.query(
          `insert into device_commands(
             id,project_id,team_id,device_id,live_stream_id,command_key,idempotency_key,
             capability_code,parameters_json,safety_context_json,status,priority,deadline_at,requested_by_user_id
           ) values($1,$2,$3,$4,$5,'start',$6,'stream.video.control',$7,$8,'dispatchable',20,now()+interval '30 seconds',$9)`,
          [commandId, projectId, access.teamId, deviceId, session.id, `live-stream:${session.id}:start`,
            { url_type: 1, url: vendorIngestURL, video_id: vendorStreamRef, video_quality: 3 },
            { liveStreamId: session.id, serverDerivedDestination: true }, user.id]
        );
        await publishProjectEvent(client, {
          projectId, teamId: access.teamId, eventId: `device.command.dispatch:${commandId}`,
          eventType: "device.command.dispatch", payload: { commandId }, enqueue: true
        });
      }
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
      if (session.status === "stopping") {
        if (!result.rows[0].vendorStreamRef) throw new Error("DJI_LIVE_VIDEO_ID_MISSING");
        const commandId = randomUUID();
        await client.query(
          `insert into device_commands(
             id,project_id,team_id,device_id,live_stream_id,command_key,idempotency_key,
             capability_code,parameters_json,safety_context_json,status,priority,deadline_at,requested_by_user_id
           ) values($1,$2,$3,$4,$5,'stop',$6,'stream.video.control',$7,$8,'dispatchable',30,now()+interval '30 seconds',$9)
           on conflict(live_stream_id,command_key) where live_stream_id is not null do nothing`,
          [commandId, projectId, access.teamId, session.deviceId, streamId, `live-stream:${streamId}:stop`,
            { video_id: result.rows[0].vendorStreamRef }, { liveStreamId: streamId }, user.id]
        );
        await publishProjectEvent(client, {
          projectId, teamId: access.teamId, eventId: `device.command.dispatch:${commandId}`,
          eventType: "device.command.dispatch", payload: { commandId }, enqueue: true
        });
      }
      await publishProjectEvent(client, {
        projectId, teamId: access.teamId, eventId: randomUUID(),
        eventType: session.status === "stopping" ? "live_stream.stop_requested" : "live_stream.stopped",
        payload: { streamId, deviceId: session.deviceId, status: session.status }, enqueue: false
      });
      return { session, replayed: false };
    }
  );
}

export async function getLiveStreamPlayback(projectId: number, streamId: number) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const result = await query<LiveStreamRow & {
    deviceTypeId: string; capabilityCode: string | null; hlsBaseURL: string | null; webrtcBaseURL: string | null;
  }>(
    `select ${qualifiedProjection},device.device_type_id::text as "deviceTypeId",
            channel.capability_code as "capabilityCode",
            profile.media_playback_base_url as "hlsBaseURL",
            profile.config_json->>'webrtcPlaybackBaseUrl' as "webrtcBaseURL"
       from live_streams stream
       join devices device on device.id=stream.device_id and device.project_id=stream.project_id
       left join device_stream_channels channel on channel.id=stream.stream_channel_id and channel.project_id=stream.project_id
       left join device_adapters adapter on adapter.id=stream.adapter_id and adapter.project_id=stream.project_id
       left join device_network_profiles profile on profile.id=adapter.network_profile_id and profile.project_id=adapter.project_id
      where stream.project_id = $1 and stream.id = $2`, [projectId, streamId]
  );
  const row = result.rows[0];
  if (!row) throw new Error("LIVE_STREAM_NOT_FOUND");
  const action = row.capabilityCode ?? "camera.live";
  if (action.startsWith("stream.")) {
    const grantRows = await query<{ actionPattern: string; effect: "allow" | "deny" }>(
      `select action_pattern as "actionPattern",effect from device_capability_grants
        where project_id=$1 and team_id=$2 and user_id=$3 and (expires_at is null or expires_at>now())
          and (scope_type='project' or (scope_type='device_type' and device_type_id=$4::bigint)
               or (scope_type='device' and device_id=$5))`,
      [projectId, access.teamId, user.id, row.deviceTypeId, row.deviceId]
    );
    authorizeCapabilityAction({ role: access.role, action,
      grants: grantRows.rows.filter((grant) => actionPatternMatches(grant.actionPattern, action)) });
  }
  const session = publicSession(row);
  assertLiveStreamProjectScope(session, projectId);
  const availability = playbackAvailability(session, new Date(), row.playbackLocatorExpiresAt);
  if (!availability.available) return { session, available: false, reason: availability.reason };
  if (session.sourceType !== "simulator" || !session.playbackRef) {
    if (session.sourceType !== "dji" || !session.playbackRef) {
      return { session, available: false, reason: "playback-adapter-unavailable" };
    }
    const protocols = [row.webrtcBaseURL ? "webrtc" : null, row.hlsBaseURL ? "hls" : null]
      .filter((value): value is "webrtc" | "hls" => Boolean(value));
    if (protocols.length === 0) return { session, available: false, reason: "playback-protocol-unavailable" };
    const issued = new MediaPlaybackTokenIssuer(getWebRuntimeConfig().authSecret).issue({
      projectId, streamId, path: session.playbackRef, protocols, ttlSeconds: 60
    });
    const candidates = buildPlaybackCandidates({ path: session.playbackRef, token: issued.token,
      hlsBaseURL: row.hlsBaseURL, webrtcBaseURL: row.webrtcBaseURL });
    await query(`update live_streams set playback_locator_expires_at=$3,updated_at=now()
      where project_id=$1 and id=$2`, [projectId, streamId, issued.expiresAt]);
    return { session, available: true, playback: { candidates, expiresAt: issued.expiresAt } };
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
