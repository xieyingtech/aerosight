import assert from "node:assert/strict";
import test from "node:test";

import { overviewSelectionHref, realtimeDeviceHref } from "./overview-map-core.ts";

test("overview maps only device selections to realtime deep links", () => {
  assert.equal(realtimeDeviceHref(7, { lane: "device-drone", entityId: "12", label: "Drone" }), "/projects/7/realtime?deviceId=12");
  assert.equal(realtimeDeviceHref(7, { lane: "alert", entityId: "12", label: "Alert" }), null);
  assert.equal(realtimeDeviceHref(7, { lane: "device-dock", entityId: "foreign", label: "Dock" }), null);
});

test("overview issue selection links to the project-scoped case", () => {
  assert.deepEqual(overviewSelectionHref(17, { lane: "issue", entityId: "42", label: "案件" }), {
    href: "/projects/17/issues/42", label: "查看案件"
  });
  assert.equal(overviewSelectionHref(17, { lane: "issue", entityId: "other", label: "案件" }), null);
});
