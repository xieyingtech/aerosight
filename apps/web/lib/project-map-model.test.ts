import assert from "node:assert/strict";
import test from "node:test";
import { createProjectMapModel, filterProjectMapModelByTime, projectMapLayers } from "./project-map-model.ts";
import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";

const snapshot: ProjectSituationSnapshot = {
  project: { id: 7, name: "North", teamId: 1 }, generatedAt: "2026-08-24T10:00:00Z", consistency: "repeatable-read",
	devices: [
		{ id: 1, projectId: 7, name: "Matrice 3TD", type: "aircraft", category: "aircraft", typeKey: "dji.matrice3td", status: "online", pose: { longitude: 120.1, latitude: 30.2 } },
		{ id: 2, projectId: 7, name: "ROS", type: "ground_robot", status: "online", pose: { longitude: 120.2, latitude: 30.3 } },
		{ id: 3, projectId: 7, name: "Dock 2", type: "dock", category: "dock", typeKey: "dji.dock2", status: "online", pose: { longitude: 120.11, latitude: 30.21 } },
		{ id: 4, projectId: 7, name: "UAV alias", type: "uav", status: "online", pose: { longitude: 120.12, latitude: 30.22 } },
		{ id: 5, projectId: 7, name: "Drone alias", type: "drone", status: "online", pose: { longitude: 120.13, latitude: 30.23 } },
		{ id: 99, projectId: 8, name: "Foreign", type: "drone", pose: { longitude: 121, latitude: 31 } }
  ],
  tracks: [{ deviceId: 1, geometry: { type: "LineString", coordinates: [[120.1, 30.2], [120.2, 30.3]] } }],
  activeTasks: [{ id: 3, taskName: "Route", status: "running", input: { route: { type: "LineString", coordinates: [[120, 30], [121, 31]] } } }],
  taskSteps: [], algorithmRuns: [],
  regions: [{ id: 4, name: "Area", geometry: { type: "Polygon", coordinates: [[[120, 30], [121, 30], [121, 31], [120, 30]]] } }],
  mediaPoints: [{ id: 5, kind: "image", metadata: { longitude: 120.3, latitude: 30.4 } }],
  suspectedConstruction: [{ id: 6, label: "疑似违建", geometry: { type: "Point", coordinates: [120.4, 30.5] } }],
  openIssues: [{ id: 7, number: 19, title: "Issue", geometry: { type: "Point", coordinates: [120.5, 30.6] } }],
  openAlerts: [], liveStreams: [],
  freshness: { latestCapturedAt: null, isRealtime: false }, availability: {}
};

test("map layer registry keeps all operational layers stable", () => {
  assert.deepEqual(projectMapLayers.map((layer) => layer.id), [
    "regions", "mission-routes", "tracks", "suspected-construction", "media", "issues", "drones", "docks", "ground-robots"
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
  for (const kind of ["device-drone", "device-ground", "track", "mission-route", "region", "media", "suspected-construction", "issue"]) {
    assert(kinds.has(kind as never), `missing ${kind}`);
  }
  assert(!model.features.some((item) => item.properties.entityId === "99"));
	assert(model.features.every((item) => item.properties.projectId === 7));
});

test("Dock 2 and M3TD snapshot uses facility and aircraft map icons", () => {
	const model = createProjectMapModel(snapshot);
	const devices = new Map(model.features.filter((item) => item.properties.layerKind.startsWith("device-"))
		.map((item) => [item.properties.entityId, item.properties]));
	assert.deepEqual(
		[devices.get("1"), devices.get("4"), devices.get("5")].map((item) => [item?.layerKind, item?.markerKind, item?.markerGlyph]),
		Array.from({ length: 3 }, () => ["device-drone", "drone", "✈"])
	);
	assert.deepEqual(
		[devices.get("3")?.layerKind, devices.get("3")?.markerKind, devices.get("3")?.markerGlyph],
		["device-dock", "dock", "▣"]
	);
});
