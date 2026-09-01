import assert from "node:assert/strict";
import test from "node:test";

import type { DeviceTreeNode } from "./device-tree-core.ts";
import { filterDeviceTree, flattenDeviceTree } from "./device-manager-core.ts";

const device = (id: number, name: string, children: DeviceTreeNode[] = []): DeviceTreeNode => ({
  id, deviceTypeId: String(id), name, category: id === 1 ? "dock" : "aircraft", status: "online",
  dataFreshness: "fresh", statusReason: null, typeName: name, typeKey: `fixture.${name}`,
  positionStatus: "missing", positionReason: "position_missing", positionSource: "fixture.driver", pose: null,
  driverKey: "fixture.driver", driverVersion: "1.0.0", vendor: "Fixture", model: null,
  capabilities: [], channels: [], relationType: id === 1 ? null : "contains", children
});

test("flattens a device topology in display order", () => {
  const tree = [device(1, "机场", [device(2, "无人机")]), device(3, "摄像头")];
  assert.deepEqual(flattenDeviceTree(tree).map((item) => item.id), [1, 2, 3]);
});

test("search keeps matching descendants and their ancestors", () => {
  const tree = [device(1, "机场", [device(2, "Matrice 3D")]), device(3, "巡检摄像头")];
  const result = filterDeviceTree(tree, "Matrice");
  assert.deepEqual(result.map((item) => item.id), [1]);
  assert.deepEqual(result[0]?.children.map((item) => item.id), [2]);
});
