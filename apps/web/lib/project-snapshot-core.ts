import type { PoolClient } from "pg";
import { dependencyHealthFromRecord, evaluateProjectHealth, type ProjectHealth } from "./dependency-health-core.ts";

type SnapshotClient = Pick<PoolClient, "query" | "release">;
export type ConnectSnapshotClient = () => Promise<SnapshotClient>;

export type ProjectSituationSnapshot = {
  project: { id: number; name: string; teamId: number; dependencyHealth?: Record<string, unknown> };
  generatedAt: string;
  consistency: "repeatable-read";
  devices: Array<Record<string, unknown>>;
  tracks: Array<Record<string, unknown>>;
  activeTasks: Array<Record<string, unknown>>;
  liveStreams: Array<Record<string, unknown>>;
  mediaPoints: Array<Record<string, unknown>>;
  suspectedConstruction: Array<Record<string, unknown>>;
  openAlerts: Array<Record<string, unknown>>;
  regions: Array<Record<string, unknown>>;
  freshness: { latestCapturedAt: string | null; isRealtime: boolean };
  availability: Record<string, "available" | "not-configured" | "degraded">;
  health?: ProjectHealth;
};

function latestTimestamp(collections: Array<Array<Record<string, unknown>>>) {
  let latest = 0;
  for (const collection of collections) {
    for (const item of collection) {
      for (const key of ["capturedAt", "updatedAt", "createdAt", "lastSeenAt"]) {
        const value = item[key];
        if (value instanceof Date) latest = Math.max(latest, value.getTime());
        else if (typeof value === "string") latest = Math.max(latest, Date.parse(value) || 0);
      }
    }
  }
  return latest ? new Date(latest) : null;
}

export async function readProjectSituationSnapshot(
  userId: number,
  projectId: number,
  connect: ConnectSnapshotClient
): Promise<ProjectSituationSnapshot | null> {
  const client = await connect();
  try {
    await client.query("begin isolation level repeatable read read only");
    const projectResult = await client.query<{
      id: number; name: string; teamId: number; dependencyHealth: Record<string, unknown>;
    }>(
      `/* snapshot:project-scope */
       select project.id, project.name, project.team_id as "teamId",
              coalesce(flags.dependency_health_json, '{}'::jsonb) as "dependencyHealth"
       from projects project
       join team_members membership on membership.team_id = project.team_id
       left join project_feature_flags flags on flags.project_id = project.id
       where project.id = $1 and membership.user_id = $2`,
      [projectId, userId]
    );
    const project = projectResult.rows[0];
    if (!project) {
      await client.query("commit");
      return null;
    }

    const devices = (await client.query<Record<string, unknown>>(
      `/* snapshot:devices */
       select device.id, device.name, device.type, device.status,
              device_type.type_key as "typeKey", device_type.version as "typeVersion",
              device_type.display_name as "typeName", device_type.category,
              driver.driver_key as "driverKey", driver.version as "driverVersion",
              driver.status as "driverStatus",
              device.status_reason as "statusReason", device.last_seen_at as "lastSeenAt",
              case when pose.observation_id is null then null else json_build_object(
                'longitude', ST_X(pose.standard_position),
                'latitude', ST_Y(pose.standard_position),
                'altitudeMeters', ST_Z(pose.standard_position),
                'capturedAt', pose.captured_at,
                'spatialQuality', pose.spatial_quality,
                'horizontalAccuracyMeters', pose.horizontal_accuracy_m
              ) end as pose
       from devices device
       join device_types device_type on device_type.id = device.device_type_id
       join driver_definitions driver on driver.id = device_type.driver_definition_id
       left join lateral (
         select observation_id, standard_position, captured_at, spatial_quality, horizontal_accuracy_m
         from poses where poses.project_id = $1 and poses.device_id = device.id
         order by captured_at desc limit 1
       ) pose on true
       where device.project_id = $1 order by device.id`,
      [projectId]
    )).rows;
    const activeTasks = (await client.query<Record<string, unknown>>(
      `/* snapshot:active-tasks */
       select run.id, run.status, run.started_at as "startedAt", run.created_at as "createdAt",
              task.id as "taskId", task.name as "taskName", run.input_snapshot_json as input
       from task_runs run join tasks task on task.id = run.task_id and task.project_id = run.project_id
       where run.project_id = $1 and run.status in ('queued', 'blocked', 'ready', 'dispatching', 'running', 'paused', 'canceling')
       order by run.created_at desc`,
      [projectId]
    )).rows;
    const tracks = (await client.query<Record<string, unknown>>(
      `/* snapshot:tracks */
       select recent.device_id as "deviceId", min(recent.captured_at) as "startedAt",
              max(recent.captured_at) as "endedAt", count(*)::int as "pointCount",
              ST_AsGeoJSON(ST_MakeLine(recent.standard_position order by recent.captured_at))::json as geometry
       from (
         select pose.device_id, pose.captured_at, pose.standard_position
         from poses pose where pose.project_id = $1 and pose.standard_position is not null
         order by pose.captured_at desc limit 5000
       ) recent
       group by recent.device_id having count(*) >= 2`,
      [projectId]
    )).rows;
    const liveStreams = (await client.query<Record<string, unknown>>(
      `/* snapshot:live-streams */
       select stream.id, stream.device_id as "deviceId", stream.stream_key as "streamKey",
              stream.source_type as "sourceType", stream.status,
              stream.status_reason as "statusReason", stream.started_at as "startedAt",
              stream.last_active_at as "lastActiveAt", stream.ended_at as "endedAt"
       from live_streams stream
       where stream.project_id = $1
         and stream.status in ('starting', 'live', 'degraded')
       order by stream.started_at desc`,
      [projectId]
    )).rows;
    const mediaPoints = (await client.query<Record<string, unknown>>(
      `/* snapshot:media */
       select asset.id, asset.kind, asset.mime_type as "mimeType", asset.device_id as "deviceId",
              asset.task_run_id as "taskRunId", asset.captured_at as "capturedAt",
              asset.created_at as "createdAt", asset.metadata_json as metadata
       from assets asset where asset.project_id = $1 and asset.status = 'available'
       order by coalesce(asset.captured_at, asset.created_at) desc limit 500`,
      [projectId]
    )).rows;
    const suspectedConstruction = (await client.query<Record<string, unknown>>(
      `/* snapshot:suspected-construction */
       select group_row.id, group_row.project_id as "projectId", '疑似违建' as label,
              group_row.status, group_row.location_quality as "locationQuality",
              group_row.last_detected_at as "capturedAt", ST_AsGeoJSON(group_row.geographic_geometry)::json as geometry
       from detection_groups group_row where group_row.project_id=$1 and group_row.status='active'
       order by group_row.last_detected_at desc limit 500`,[projectId]
    )).rows;
    const openAlerts = (await client.query<Record<string, unknown>>(
      `/* snapshot:alerts */
       select event.id,event.project_id as "projectId",'疑似违建' as title,event.status,event.severity,
              event.last_detected_at as "updatedAt",ST_AsGeoJSON(group_row.geographic_geometry)::json as geometry
       from perception_events event join detection_groups group_row on group_row.id=event.detection_group_id and group_row.project_id=event.project_id
       where event.project_id=$1 and event.status in('open','acknowledged','investigating')
       order by event.last_detected_at desc limit 500`,
      [projectId]
    )).rows;

    const generatedAt = new Date();
    const latest = latestTimestamp([devices, tracks, activeTasks, liveStreams, mediaPoints, openAlerts]);
    const health = evaluateProjectHealth(dependencyHealthFromRecord(project.dependencyHealth));
    const snapshot: ProjectSituationSnapshot = {
      project,
      generatedAt: generatedAt.toISOString(),
      consistency: "repeatable-read",
      devices,
      tracks,
      activeTasks,
      liveStreams,
      mediaPoints,
      suspectedConstruction,
      openAlerts,
      regions: [],
      freshness: {
        latestCapturedAt: latest?.toISOString() ?? null,
        isRealtime: Boolean(latest && generatedAt.getTime() - latest.getTime() <= 120_000)
      },
      availability: {
        devices: "available", tasks: "available", media: "available", alerts: "available",
        liveStreams: health.capabilityAvailability.realtime_device_control === "degraded" ? "degraded" : "available",
        suspectedConstruction: health.capabilityAvailability.algorithm_execution === "degraded" ? "degraded" : "available",
        regions: "not-configured"
      },
      health
    };
    await client.query("commit");
    return snapshot;
  } catch (error) {
    await client.query("rollback");
    throw error;
  } finally {
    client.release();
  }
}
