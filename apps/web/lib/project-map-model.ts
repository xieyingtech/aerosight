import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";
import { presentDevicePosition } from "./device-position-presentation.ts";

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
  { id: "algorithm-results", label: "算法识别结果", kind: "algorithm-results" },
  { id: "media", label: "媒体点", kind: "media" },
  { id: "issues", label: "案件", kind: "issue" },
  { id: "devices", label: "设备", kind: "device-generic" }
] as const;

type MapProperties = {
  projectId: number;
  layerKind: typeof projectMapLayers[number]["kind"];
  entityId: string;
  label: string;
  status?: string;
  warningSeverity?: "warning" | "error";
  warningMessage?: string;
  positionLabel?: string;
  ownerDeviceTypeKey?: string;
  deviceTypeKey?: string;
  deviceTypeName?: string;
  deviceTypeIcon?: string;
	capturedAt?: string;
	dataFreshness?: string;
	positionStatus?: string;
	positionReason?: string;
	positionSource?: string;
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

function deviceTypePresentation(device: ProjectSituationSnapshot["devices"][number]) {
  return {
    deviceTypeKey: device.deviceTypeId || device.typeKey || "unknown",
    deviceTypeName: device.typeName || "未设置设备类型",
    deviceTypeIcon: device.typeIcon || undefined
  };
}

export function createProjectMapModel(snapshot: ProjectSituationSnapshot): FeatureCollection<Geometry, MapProperties> {
  const projectId = snapshot.project.id;
  const features: Array<Feature<Geometry, MapProperties>> = [];
  for (const device of snapshot.devices) {
    if (!scoped(device, projectId)) continue;
		const pose = device.pose as Record<string, unknown> | null;
		const position = pose && point(pose.longitude, pose.latitude);
		if (!position) continue;
		const appearance = deviceTypePresentation(device);
		const presentedPosition = presentDevicePosition(device);
		const diagnostics = (snapshot.diagnostics ?? []).filter(item => Number(item.deviceId) === Number(device.id) && item.deviceId != null && item.severity !== "info");
		const warningSeverity = diagnostics.some(item => item.severity === "error") || presentedPosition.state === "invalid" ? "error"
			: diagnostics.length || ["unverified", "stale"].includes(presentedPosition.state) ? "warning" : undefined;
		const warningMessage = [...diagnostics.map(item => item.title), ...(presentedPosition.state !== "available" ? [presentedPosition.label] : [])].join("\n");
		features.push(feature(projectId, position, {
			...appearance, layerKind: "device-generic", entityId: String(device.id), label: String(device.name ?? "未命名设备"),
			status: String(device.status ?? "unknown"), capturedAt: pose.capturedAt ? String(pose.capturedAt) : undefined,
			dataFreshness: String(device.dataFreshness ?? "unknown"), positionStatus: presentedPosition.state,
			positionReason: presentedPosition.reason, positionSource: presentedPosition.source,
			positionLabel: presentedPosition.label, warningSeverity, warningMessage
		}));
  }
  for (const track of snapshot.tracks) {
    if (!scoped(track, projectId)) continue;
    const line = geometry(track.geometry);
    if (line?.type !== "LineString") continue;
    const owner = snapshot.devices.find(device => Number(device.id) === Number(track.deviceId));
    features.push(feature(projectId, line, {
      layerKind: "track", entityId: String(track.deviceId), label: `设备 ${track.deviceId} 轨迹`,
      ownerDeviceTypeKey: owner ? deviceTypePresentation(owner).deviceTypeKey : undefined,
      capturedAt: track.endedAt ? String(track.endedAt) : undefined
    }));
  }
  for (const task of snapshot.activeTasks) {
    if (!scoped(task, projectId)) continue;
    const input = task.input as Record<string, unknown> | undefined;
    const owner = snapshot.devices.find(device => Number(device.id) === Number(input?.deviceId ?? task.deviceId));
    const route = geometry(input?.route);
    if (route?.type === "LineString") features.push(feature(projectId, route, {
      layerKind: "mission-route", entityId: String(task.id), label: String(task.taskName ?? "任务航线"), status: String(task.status ?? ""),
      ownerDeviceTypeKey: owner ? deviceTypePresentation(owner).deviceTypeKey : undefined
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
  for (const item of snapshot.algorithmResults) {
    if (!scoped(item, projectId)) continue;
    const shape = geometry(item.geometry);
    if (shape) features.push(feature(projectId, shape, {
      layerKind: "algorithm-results", entityId: String(item.id), label: String(item.label ?? "算法识别结果"), status: String(item.status ?? "open")
    }));
  }
  for (const item of snapshot.openIssues) {
    if (!scoped(item, projectId)) continue;
    const position = geometry(item.geometry) ?? point(item.longitude, item.latitude);
    if (position) features.push(feature(projectId, position, {
      layerKind: "issue", entityId: String(item.id), label: `#${String(item.number ?? "—")} ${String(item.title ?? "案件")}`, status: String(item.status ?? "open"),
      warningSeverity: ["urgent", "high", "critical", "error"].includes(String(item.priority ?? item.severity)) ? "error" : "warning",
      warningMessage: String(item.title ?? "待处理案件")
    }));
  }
  return { type: "FeatureCollection", features };
}

export function projectMapLayerCounts(model: ReturnType<typeof createProjectMapModel>) {
  return Object.fromEntries(projectMapLayers.map(layer => [layer.id,
    new Set(model.features.filter(item => item.properties.layerKind === layer.kind).map(item => item.properties.entityId)).size
  ])) as Record<typeof projectMapLayers[number]["id"], number>;
}

export function filterProjectMapModelByLayers(model: ReturnType<typeof createProjectMapModel>, visible: ReadonlySet<typeof projectMapLayers[number]["id"]>) {
  const kinds = new Set(projectMapLayers.filter(layer => visible.has(layer.id)).map(layer => layer.kind));
  return { ...model, features: model.features.filter(item => kinds.has(item.properties.layerKind)) };
}

export function firstMapCoordinate(model: FeatureCollection<Geometry, MapProperties>): [number, number] | null {
  for (const item of model.features) {
    if (item.geometry.type === "Point") return item.geometry.coordinates.slice(0, 2) as [number, number];
    if (item.geometry.type === "LineString") return (item.geometry as LineString).coordinates[0]?.slice(0, 2) as [number, number] ?? null;
  }
  return null;
}

export function filterProjectMapModelByTime(
  model: FeatureCollection<Geometry, MapProperties>,
  range: { from: string; to: string } | null
): FeatureCollection<Geometry, MapProperties> {
  if (!range) return model;
  return {
    ...model,
    features: model.features.filter((item) => !item.properties.capturedAt ||
      (item.properties.capturedAt >= range.from && item.properties.capturedAt <= range.to))
  };
}

export function projectMapDeviceTypes(model: ReturnType<typeof createProjectMapModel>) {
  const types = new Map<string, { key: string; label: string; icon?: string; count: number }>();
  for (const item of model.features) {
    if (item.properties.layerKind !== "device-generic") continue;
    const key = item.properties.deviceTypeKey!;
    const current = types.get(key);
    if (current) current.count++;
    else types.set(key, { key, label: item.properties.deviceTypeName!, icon: item.properties.deviceTypeIcon, count: 1 });
  }
  return [...types.values()];
}

export function filterProjectMapModelByDeviceTypes(model: ReturnType<typeof createProjectMapModel>, hiddenTypes: ReadonlySet<string>) {
  return { ...model, features: model.features.filter(item =>
    !hiddenTypes.has(item.properties.deviceTypeKey ?? "") && !hiddenTypes.has(item.properties.ownerDeviceTypeKey ?? "")) };
}
