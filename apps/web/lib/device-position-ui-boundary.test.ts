import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path: string) => readFileSync(new URL(`../../../${path}`, import.meta.url), "utf8");

test("device details render position, freshness, source, and unavailable reason", () => {
  const component = read("apps/web/components/device-tree.tsx");
  assert.match(component, /presentDevicePosition\(device\)/);
  for (const label of ["位置与数据来源", "采集时间", "数据新鲜度", "来源"]) assert.match(component, new RegExp(label));
  assert.match(component, /position\.reason/);
});

test("project map exposes original unverified poses without claiming calibration", () => {
  const snapshot = read("apps/web/lib/project-snapshot-core.ts");
  const map = read("apps/web/components/project-map.tsx");
  assert.match(snapshot, /coalesce\(pose\.standard_position,pose\.original_position\)/);
  assert.match(snapshot, /coordinate_reference_unverified/);
  assert.match(snapshot, /calibrationStatus/);
  assert.match(map, /未校准/);
  assert.match(map, /positionStatus/);
});
