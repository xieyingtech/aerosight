import assert from "node:assert/strict";
import test from "node:test";

import { actionsForCapability } from "./device-capability-actions.ts";

test("server capability catalog drives workflows, control, live, and generic sensor parameters", () => {
  assert.equal(actionsForCapability("mission.execute", "high")[0]?.kind, "workflow");
  assert.equal(actionsForCapability("flight.return_home", "critical")[0]?.key, "return_home");
  assert.ok(actionsForCapability("dock.debug.control", "critical").some((action) => action.key === "cover.open"));
  assert.equal(actionsForCapability("stream.video.control", "medium")[0]?.kind, "live");
  assert.deepEqual(actionsForCapability("sensor.configure", "medium")[0]?.fields.map((field) => field.key),
    ["sample_interval_seconds", "report_threshold"]);
  assert.deepEqual(actionsForCapability("vendor.unknown", "low"), []);
});
