import assert from "node:assert/strict";
import test from "node:test";
import { featureLeaves, featureSelection, featureChanges, type FeatureNode } from "./project-features.ts";
const tree: FeatureNode = { id: "live", label: "直播", children: [
  { id: "live.control", label: "启动" },
  { id: "converter", label: "转换", children: [{ id: "converter.create", label: "创建" }, { id: "converter.delete", label: "删除" }] },
] };
test("nested feature groups derive mixed state from leaves without inherited grants", () => {
  assert.deepEqual(featureLeaves(tree), ["live.control", "converter.create", "converter.delete"]);
  assert.equal(featureSelection(tree, {}), "none");
  assert.equal(featureSelection(tree, { "live.control": true }), "some");
  assert.equal(featureSelection(tree, Object.fromEntries(featureLeaves(tree).map(id => [id, true]))), "all");
  assert.equal(featureSelection(tree, { live: true, converter: true }), "none");
});
test("save sends only changed known leaves with expected values", () => {
  assert.deepEqual(featureChanges({ "live.control": false, "flight.execute": true }, { "live.control": true, "flight.execute": true, userId: true }), {
    "live.control": { expected: false, enabled: true },
  });
});
