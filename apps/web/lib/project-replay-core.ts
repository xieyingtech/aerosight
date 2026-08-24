import type { PoolClient } from "pg";

type ReplayClient = Pick<PoolClient, "query" | "release">;
export type ReplayConnect = () => Promise<ReplayClient>;

export type ReplayQuery = {
  from: string;
  to: string;
  deviceTypes: string[];
  bbox: [number, number, number, number] | null;
};

export type ProjectReplay = {
  projectId: number;
  mode: "replay";
  window: { from: string; to: string };
  filters: { deviceTypes: string[]; bbox: ReplayQuery["bbox"] };
  poses: Array<Record<string, unknown>>;
  media: Array<Record<string, unknown>>;
  events: Array<Record<string, unknown>>;
  truncated: boolean;
};

export function parseReplayQuery(url: URL, now = new Date()): ReplayQuery {
  const to = new Date(url.searchParams.get("to") ?? now);
  const from = new Date(url.searchParams.get("from") ?? to.getTime() - 60 * 60_000);
  if (!Number.isFinite(from.getTime()) || !Number.isFinite(to.getTime()) || from >= to) throw new Error("INVALID_REPLAY_WINDOW");
  if (to.getTime() - from.getTime() > 7 * 24 * 60 * 60_000) throw new Error("REPLAY_WINDOW_TOO_LARGE");
  const deviceTypes = (url.searchParams.get("deviceTypes") ?? "").split(",").map((value) => value.trim()).filter(Boolean);
  const bboxText = url.searchParams.get("bbox");
  let bbox: ReplayQuery["bbox"] = null;
  if (bboxText) {
    const values = bboxText.split(",").map(Number);
    if (values.length !== 4 || values.some((value) => !Number.isFinite(value)) || values[0] >= values[2] || values[1] >= values[3] || values[0] < -180 || values[2] > 180 || values[1] < -90 || values[3] > 90) throw new Error("INVALID_REPLAY_BBOX");
    bbox = values as ReplayQuery["bbox"];
  }
  return { from: from.toISOString(), to: to.toISOString(), deviceTypes, bbox };
}

export async function readProjectReplay(userId: number, projectId: number, input: ReplayQuery, connect: ReplayConnect): Promise<ProjectReplay | null> {
  const client = await connect();
  try {
    await client.query("begin isolation level repeatable read read only");
    const access = await client.query(
      `/* replay:project-scope */ select 1 from projects project
       join team_members membership on membership.team_id = project.team_id
       where project.id = $1 and membership.user_id = $2`, [projectId, userId]
    );
    if (!access.rowCount) { await client.query("commit"); return null; }
    const values: unknown[] = [projectId, input.from, input.to, input.deviceTypes, input.bbox];
    const poses = (await client.query<Record<string, unknown>>(
      `/* replay:poses */
       select pose.observation_id as id, pose.device_id as "deviceId", device.name as "deviceName",
              device.type as "deviceType", pose.captured_at as "capturedAt", pose.spatial_quality as "spatialQuality",
              ST_X(pose.standard_position) as longitude, ST_Y(pose.standard_position) as latitude,
              ST_Z(pose.standard_position) as "altitudeMeters"
       from poses pose join devices device on device.id = pose.device_id and device.project_id = pose.project_id
       where pose.project_id = $1 and pose.captured_at >= $2 and pose.captured_at <= $3
         and pose.standard_position is not null
         and (cardinality($4::text[]) = 0 or device.type = any($4::text[]))
         and ($5::double precision[] is null or ST_Intersects(
           pose.standard_position,
           ST_MakeEnvelope(($5::double precision[])[1], ($5::double precision[])[2], ($5::double precision[])[3], ($5::double precision[])[4], 4326)
         ))
       order by pose.captured_at limit 5001`, values
    )).rows;
    const media = (await client.query<Record<string, unknown>>(
      `/* replay:media */ select id, device_id as "deviceId", kind, mime_type as "mimeType",
              captured_at as "capturedAt", metadata_json as metadata
       from assets where project_id = $1 and captured_at >= $2 and captured_at <= $3
       order by captured_at limit 1001`, [projectId, input.from, input.to]
    )).rows;
    const events = (await client.query<Record<string, unknown>>(
      `/* replay:events */ select cursor::text, event_type as "eventType", payload_json as payload,
              occurred_at as "occurredAt"
       from project_events where project_id = $1 and occurred_at >= $2 and occurred_at <= $3
       order by cursor limit 2001`, [projectId, input.from, input.to]
    )).rows;
    const truncated = poses.length > 5000 || media.length > 1000 || events.length > 2000;
    await client.query("commit");
    return { projectId, mode: "replay", window: { from: input.from, to: input.to }, filters: { deviceTypes: input.deviceTypes, bbox: input.bbox }, poses: poses.slice(0, 5000), media: media.slice(0, 1000), events: events.slice(0, 2000), truncated };
  } catch (error) {
    await client.query("rollback");
    throw error;
  } finally { client.release(); }
}
