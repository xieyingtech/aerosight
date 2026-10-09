import assert from "node:assert/strict";
import test from "node:test";
import { createProjectMapModel, projectMapDeviceTypes, filterProjectMapModelByDeviceTypes, filterProjectMapModelByLayers, filterProjectMapModelByTime, projectMapLayerCounts, projectMapLayers } from "./project-map-model.ts";
import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";

const snapshot: ProjectSituationSnapshot = {
  project: { id: 7, name: "North", teamId: 1 }, generatedAt: "2026-08-24T10:00:00Z", consistency: "repeatable-read",
	devices: [
		{ id: 1, projectId: 7, name: "Matrice 3TD", type: "aircraft", category: "aircraft", typeKey: "dji.matrice3td", status: "online", dataFreshness: "fresh", positionStatus: "unverified", positionReason: "coordinate_reference_unverified", positionSource: "dji-flighthub-openapi", pose: { longitude: 120.1, latitude: 30.2, calibrationStatus: "unverified" } },
		{ id: 2, projectId: 7, name: "ROS", type: "ground_robot", status: "online", pose: { longitude: 120.2, latitude: 30.3 } },
		{ id: 3, projectId: 7, name: "Dock 2", type: "dock", category: "dock", typeKey: "dji.dock2", status: "online", dataFreshness: "fresh", positionStatus: "unverified", positionSource: "dji-flighthub-openapi", pose: { longitude: 120.11, latitude: 30.21, calibrationStatus: "unverified" } },
		{ id: 4, projectId: 7, name: "UAV alias", type: "uav", status: "online", pose: { longitude: 120.12, latitude: 30.22 } },
		{ id: 5, projectId: 7, name: "Drone alias", type: "drone", status: "online", pose: { longitude: 120.13, latitude: 30.23 } },
		{ id: 99, projectId: 8, name: "Foreign", type: "drone", pose: { longitude: 121, latitude: 31 } }
  ],
  tracks: [{ deviceId: 1, geometry: { type: "LineString", coordinates: [[120.1, 30.2], [120.2, 30.3]] } }],
  activeTasks: [{ id: 3, taskName: "Route", status: "running", input: { route: { type: "LineString", coordinates: [[120, 30], [121, 31]] } } }],
  taskSteps: [], algorithmRuns: [],
  regions: [{ id: 4, name: "Area", geometry: { type: "Polygon", coordinates: [[[120, 30], [121, 30], [121, 31], [120, 30]]] } }],
  mediaPoints: [{ id: 5, kind: "image", metadata: { longitude: 120.3, latitude: 30.4 } }],
  algorithmResults: [{ id: 6, label: "车辆识别", geometry: { type: "Point", coordinates: [120.4, 30.5] } }],
  openIssues: [{ id: 7, number: 19, title: "Issue", geometry: { type: "Point", coordinates: [120.5, 30.6] } }],
  openAlerts: [], liveStreams: [],
  freshness: { latestCapturedAt: null, isRealtime: false }, availability: {}
};

test("map layer registry keeps all operational layers stable", () => {
  assert.deepEqual(projectMapLayers.map((layer) => layer.id), [
    "regions", "mission-routes", "tracks", "algorithm-results", "media", "issues", "devices"
  ]);
});

test("history window keeps timeless regions but filters timestamped features", () => {
  const model = createProjectMapModel(snapshot);
  const filtered = filterProjectMapModelByTime(model, { from: "2026-08-24T10:00:00Z", to: "2026-08-24T10:01:00Z" });
  assert(filtered.features.some((item) => item.properties.layerKind === "region"));
  assert(filtered.features.length <= model.features.length);
});

test("map model renders air-ground features and drops foreign project data", () => {
  const model = createProjectMapModel(snapshot);
  const kinds = new Set(model.features.map((item) => item.properties.layerKind));
  for (const kind of ["device-generic", "track", "mission-route", "region", "media", "algorithm-results", "issue"]) {
    assert(kinds.has(kind as never), `missing ${kind}`);
  }
  assert(!model.features.some((item) => item.properties.entityId === "99"));
	assert(model.features.every((item) => item.properties.projectId === 7));
  assert.equal(model.features.find((item) => item.properties.layerKind === "algorithm-results")?.properties.label, "车辆识别");
});

test("device types provide map names and icons without inferring air or ground categories", () => {
  const model = createProjectMapModel({ ...snapshot, devices: [
    { ...snapshot.devices[0], deviceTypeId: "42", typeName: "Custom camera", typeIcon: "camera" },
    { ...snapshot.devices[1], deviceTypeId: "43", typeName: "Sensor", typeIcon: "thermometer" },
    snapshot.devices[3]
  ] });
  const types = projectMapDeviceTypes(model);
  assert.deepEqual(types.map(type => [type.key, type.label, type.icon, type.count]), [
    ["42", "Custom camera", "camera", 1], ["43", "Sensor", "thermometer", 1], ["unknown", "未设置设备类型", undefined, 1]
  ]);
  assert(model.features.filter(item => item.properties.deviceTypeKey).every(item => item.properties.layerKind === "device-generic"));
});

test("layer badge counts match mappable project entities, including hidden layer counts", () => {
  const model = createProjectMapModel({ ...snapshot, tracks: [...snapshot.tracks, ...snapshot.tracks] });
  const counts = projectMapLayerCounts(model);
  assert.equal(counts.devices, 5);
  assert.equal(counts.tracks, 1);
  assert.equal(counts["mission-routes"], 1);
  assert.equal(counts.issues, 1);
  const history = filterProjectMapModelByTime(createProjectMapModel({ ...snapshot,
    mediaPoints: [{ ...snapshot.mediaPoints[0], capturedAt: "2026-08-23T00:00:00Z" }]
  }), { from: "2026-08-24T00:00:00Z", to: "2026-08-25T00:00:00Z" });
  assert.equal(projectMapLayerCounts(history).media, 0);
});

test("warning indicators bind diagnostics to their device and error wins over position warning", () => {
  const model = createProjectMapModel({ ...snapshot, diagnostics: [
    { id: "device-error", deviceId: 1, kind: "stream", severity: "error", title: "直播中断", reason: "failed", status: "failed", occurredAt: null },
    { id: "project-error", deviceId: null, kind: "connection", severity: "error", title: "连接异常", reason: "failed", status: "failed", occurredAt: null }
  ] });
  const drone = model.features.find(item => item.properties.layerKind === "device-generic" && item.properties.entityId === "1");
  const dock = model.features.find(item => item.properties.layerKind === "device-generic" && item.properties.entityId === "3");
  const ground = model.features.find(item => item.properties.layerKind === "device-generic" && item.properties.entityId === "2");
  assert.equal(drone?.properties.warningSeverity, "error");
  assert.match(drone?.properties.warningMessage ?? "", /直播中断/);
  assert.equal(dock?.properties.warningSeverity, "warning");
  assert.equal(ground?.properties.warningSeverity, undefined);
});

test("type visibility follows device ownership for tracks and routes, without guessing unowned routes", () => {
  const model = createProjectMapModel({ ...snapshot,
    devices: snapshot.devices.map(device => ({ ...device, deviceTypeId: String(device.id) })),
    activeTasks: [...snapshot.activeTasks, { id: 88, input: { deviceId: 1, route: { type: "LineString", coordinates: [[120, 30], [121, 31]] } } }],
    tracks: [...snapshot.tracks, { deviceId: 2, geometry: { type: "LineString", coordinates: [[120, 30], [121, 31]] } }]
  });
  const filtered = filterProjectMapModelByDeviceTypes(model, new Set(["1"]));
  assert(!filtered.features.some(item => item.properties.layerKind === "device-generic" && item.properties.entityId === "1"));
  assert(!filtered.features.some(item => item.properties.layerKind === "track" && item.properties.entityId === "1"));
  assert(!filtered.features.some(item => item.properties.layerKind === "mission-route" && item.properties.entityId === "88"));
  assert(filtered.features.some(item => item.properties.layerKind === "track" && item.properties.entityId === "2"));
  assert(filtered.features.some(item => item.properties.layerKind === "mission-route" && item.properties.entityId === "3"));
  assert.equal(filterProjectMapModelByDeviceTypes(model, new Set()).features.length, model.features.length);
});
