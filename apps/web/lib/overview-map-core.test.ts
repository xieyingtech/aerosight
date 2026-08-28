import assert from "node:assert/strict";
import test from "node:test";

import { realtimeDeviceHref } from "./overview-map-core.ts";

test("overview maps only device selections to realtime deep links", () => {
  assert.equal(realtimeDeviceHref(7, { lane: "device-drone", entityId: "12", label: "Drone" }), "/projects/7/realtime?deviceId=12");
  assert.equal(realtimeDeviceHref(7, { lane: "alert", entityId: "12", label: "Alert" }), null);
  assert.equal(realtimeDeviceHref(7, { lane: "device-dock", entityId: "foreign", label: "Dock" }), null);
});
