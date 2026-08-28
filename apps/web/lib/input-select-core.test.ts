import assert from "node:assert/strict";
import test from "node:test";

import { filterInputSelectOptions } from "./input-select-core.ts";

const options = [
  { value: "1", label: "DJI Dock 3", description: "机场", keywords: ["dji.dock3", "dji.cloud"] },
  { value: "2", label: "环境传感器", description: "sensor", keywords: ["stream.sensor.read"] }
];

test("input select searches labels, descriptions and hidden keywords", () => {
  assert.deepEqual(filterInputSelectOptions(options, "dock").map((item) => item.value), ["1"]);
  assert.deepEqual(filterInputSelectOptions(options, "SENSOR").map((item) => item.value), ["2"]);
  assert.deepEqual(filterInputSelectOptions(options, "dji.cloud").map((item) => item.value), ["1"]);
  assert.equal(filterInputSelectOptions(options, "robot").length, 0);
});
