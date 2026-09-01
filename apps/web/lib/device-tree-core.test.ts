import assert from "node:assert/strict";
import test from "node:test";

import { buildDeviceTree, type DeviceTreeItem } from "./device-tree-core.ts";

const device = (id: number, category: string): DeviceTreeItem => ({
  id, deviceTypeId: String(id), name: `${category}-${id}`, category, status: "online", dataFreshness: "fresh", statusReason: null,
  positionStatus: "missing", positionReason: "position_missing", positionSource: "fixture.driver", pose: null,
  typeName: category, typeKey: `fixture.${category}`, driverKey: "fixture.driver", driverVersion: "1.0.0",
  vendor: category === "unknown" ? null : "Fixture", model: null,
  capabilities: [{ code: "state.read", availability: "available", reason: null, risk: "low", authorized: true, actions: [] }], channels: []
});

test("all device categories render through the same topology model", () => {
  const devices = [device(1, "dock"), device(2, "aircraft"), device(3, "robot"),
    device(4, "camera"), device(5, "sensor"), device(6, "unknown")];
  const tree = buildDeviceTree(devices, [
    { fromDeviceId: 1, toDeviceId: 2, relationType: "docked-aircraft" },
    { fromDeviceId: 2, toDeviceId: 4, relationType: "mounted-on" },
    { fromDeviceId: 1, toDeviceId: 5, relationType: "contains" }
  ]);
  const flatten = (nodes: typeof tree): typeof tree => nodes.flatMap((node) => [node, ...flatten(node.children)]);
  assert.deepEqual(new Set(flatten(tree).map((item) => item.category)),
    new Set(["dock", "aircraft", "robot", "camera", "sensor", "unknown"]));
  assert.equal(tree.find((item) => item.id === 1)?.children.find((item) => item.id === 2)?.relationType, "docked-aircraft");
});

test("cyclic or malformed topology never hides a device", () => {
  const devices = [device(1, "gateway"), device(2, "camera")];
  const tree = buildDeviceTree(devices, [
    { fromDeviceId: 1, toDeviceId: 2, relationType: "contains" },
    { fromDeviceId: 2, toDeviceId: 1, relationType: "invalid-cycle" }
  ]);
  assert(tree.length > 0);
});
