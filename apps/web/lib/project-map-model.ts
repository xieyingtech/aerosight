import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";

type Point = { type: "Point"; coordinates: number[] };
type LineString = { type: "LineString"; coordinates: number[][] };
type Polygon = { type: "Polygon"; coordinates: number[][][] };
type MultiPolygon = { type: "MultiPolygon"; coordinates: number[][][][] };
type Geometry = Point | LineString | Polygon | MultiPolygon;
type Feature<G extends Geometry, P> = { type: "Feature"; geometry: G; properties: P };
type FeatureCollection<G extends Geometry, P> = { type: "FeatureCollection"; features: Array<Feature<G, P>> };

export const projectMapLayers = [
  { id: "regions", label: "巡检区域", kind: "region" },
  { id: "mission-routes", label: "任务航线", kind: "mission-route" },
  { id: "tracks", label: "运行轨迹", kind: "track" },
  { id: "suspected-construction", label: "疑似违建", kind: "suspected-construction" },
  { id: "media", label: "媒体点", kind: "media" },
  { id: "alerts", label: "告警", kind: "alert" },
  { id: "drones", label: "无人机", kind: "device-drone" },
  { id: "docks", label: "机巢", kind: "device-dock" },
  { id: "ground-robots", label: "地面设备", kind: "device-ground" }
] as const;

type MapProperties = {
  projectId: number;
  layerKind: typeof projectMapLayers[number]["kind"];
  entityId: string;
  label: string;
  status?: string;
  capturedAt?: string;
};

function scoped(item: Record<string, unknown>, projectId: number) {
  return item.projectId === undefined || Number(item.projectId) === projectId;
}

function geometry(value: unknown): Geometry | null {
  if (!value || typeof value !== "object") return null;
  const candidate = value as { type?: unknown; coordinates?: unknown };
  if (typeof candidate.type !== "string" || !Array.isArray(candidate.coordinates)) return null;
  return candidate as Geometry;
}

function point(longitude: unknown, latitude: unknown): Point | null {
  const lon = Number(longitude);
  const lat = Number(latitude);
  if (!Number.isFinite(lon) || !Number.isFinite(lat) || lon < -180 || lon > 180 || lat < -90 || lat > 90) return null;
  return { type: "Point", coordinates: [lon, lat] };
}

function feature(projectId: number, geometryValue: Geometry, properties: Omit<MapProperties, "projectId">): Feature<Geometry, MapProperties> {
  return { type: "Feature", geometry: geometryValue, properties: { projectId, ...properties } };
}

function deviceLayer(type: unknown): MapProperties["layerKind"] {
  const normalized = String(type ?? "").toLowerCase();
  if (normalized.includes("dock") || normalized.includes("nest")) return "device-dock";
  if (normalized.includes("drone") || normalized.includes("uav")) return "device-drone";
  return "device-ground";
}

export function createProjectMapModel(snapshot: ProjectSituationSnapshot): FeatureCollection<Geometry, MapProperties> {
  const projectId = snapshot.project.id;
  const features: Array<Feature<Geometry, MapProperties>> = [];
  for (const device of snapshot.devices) {
    if (!scoped(device, projectId)) continue;
    const pose = device.pose as Record<string, unknown> | null;
    const position = pose && point(pose.longitude, pose.latitude);
    if (!position) continue;
    features.push(feature(projectId, position, {
      layerKind: deviceLayer(device.type), entityId: String(device.id), label: String(device.name ?? "未命名设备"),
      status: String(device.status ?? "unknown"), capturedAt: pose.capturedAt ? String(pose.capturedAt) : undefined
    }));
  }
  for (const track of snapshot.tracks) {
    if (!scoped(track, projectId)) continue;
    const line = geometry(track.geometry);
    if (line?.type !== "LineString") continue;
    features.push(feature(projectId, line, {
      layerKind: "track", entityId: String(track.deviceId), label: `设备 ${track.deviceId} 轨迹`,
      capturedAt: track.endedAt ? String(track.endedAt) : undefined
    }));
  }
  for (const task of snapshot.activeTasks) {
    if (!scoped(task, projectId)) continue;
    const input = task.input as Record<string, unknown> | undefined;
    const route = geometry(input?.route);
    if (route?.type === "LineString") features.push(feature(projectId, route, {
      layerKind: "mission-route", entityId: String(task.id), label: String(task.taskName ?? "任务航线"), status: String(task.status ?? "")
    }));
  }
  for (const region of snapshot.regions) {
    if (!scoped(region, projectId)) continue;
    const shape = geometry(region.geometry);
    if (!shape || (shape.type !== "Polygon" && shape.type !== "MultiPolygon")) continue;
    features.push(feature(projectId, shape, { layerKind: "region", entityId: String(region.id), label: String(region.name ?? "巡检区域") }));
  }
  for (const item of snapshot.mediaPoints) {
    if (!scoped(item, projectId)) continue;
    const metadata = item.metadata as Record<string, unknown> | undefined;
    const position = point(metadata?.longitude, metadata?.latitude);
    if (position) features.push(feature(projectId, position, {
      layerKind: "media", entityId: String(item.id), label: String(item.kind ?? "媒体"), capturedAt: item.capturedAt ? String(item.capturedAt) : undefined
    }));
  }
  for (const item of snapshot.suspectedConstruction) {
    if (!scoped(item, projectId)) continue;
    const shape = geometry(item.geometry);
    if (shape) features.push(feature(projectId, shape, {
      layerKind: "suspected-construction", entityId: String(item.id), label: String(item.label ?? "疑似违建"), status: String(item.status ?? "open")
    }));
  }
  for (const item of snapshot.openAlerts) {
    if (!scoped(item, projectId)) continue;
    const position = geometry(item.geometry) ?? point(item.longitude, item.latitude);
    if (position) features.push(feature(projectId, position, {
      layerKind: "alert", entityId: String(item.id), label: String(item.title ?? "告警"), status: String(item.status ?? "open")
    }));
  }
  return { type: "FeatureCollection", features };
}

export function firstMapCoordinate(model: FeatureCollection<Geometry, MapProperties>): [number, number] | null {
  for (const item of model.features) {
    if (item.geometry.type === "Point") return item.geometry.coordinates.slice(0, 2) as [number, number];
    if (item.geometry.type === "LineString") return (item.geometry as LineString).coordinates[0]?.slice(0, 2) as [number, number] ?? null;
  }
  return null;
}
