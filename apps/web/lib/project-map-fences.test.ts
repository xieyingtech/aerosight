import { test } from "node:test";
import assert from "node:assert/strict";
import { createProjectMapFences } from "./project-map-fences.ts";

test("fences follow polygon outer and inner rings, multipolygons, and exclude detections", () => {
  const ring = [[121, 31], [121.001, 31], [121.001, 31.001], [121, 31]];
  const inner = ring.map(([x, y]) => [x + 0.0001, y + 0.0001]);
  const properties = { projectId: 1, layerKind: "region" as const, entityId: "1", label: "region" };
  const outer = createProjectMapFences({ type: "FeatureCollection", features: [{ type: "Feature", properties, geometry: { type: "Polygon", coordinates: [ring] } }] });
  const multi = createProjectMapFences({ type: "FeatureCollection", features: [
    { type: "Feature", properties, geometry: { type: "MultiPolygon", coordinates: [[ring, inner], [ring]] } },
    { type: "Feature", properties: { ...properties, layerKind: "algorithm-results" }, geometry: { type: "Polygon", coordinates: [ring] } }
  ] });
  assert.equal(multi.features.length, outer.features.length * 3);
  assert.ok(outer.features.some(f => f.properties.kind === "post"));
  for (const f of multi.features) {
    assert.deepEqual(f.geometry.coordinates[0][0], f.geometry.coordinates[0].at(-1));
    assert.ok(f.geometry.coordinates.flat(2).every(Number.isFinite));
  }
});

test("degenerate coordinates are skipped and huge rings stay bounded", () => {
  const properties = { projectId: 1, layerKind: "region" as const, entityId: "1", label: "region" };
  const build = (coordinates: number[][]) => createProjectMapFences({ type: "FeatureCollection", features: [{ type: "Feature", properties, geometry: { type: "Polygon", coordinates: [coordinates] } }] });
  assert.equal(build([[121, 31], [121, 31], [NaN, 31], [121, 90]]).features.length, 0);
  assert.ok(build(Array.from({ length: 10000 }, (_, i) => [121 + i % 2 * 0.01, 31])).features.length <= 8192);
});
