import assert from "node:assert/strict";
import test from "node:test";
import { projectNavigationHref, visibleProjectNavigation } from "./project-navigation.ts";

test("project manager sees the complete project workspace navigation", () => {
  assert.deepEqual(
    visibleProjectNavigation("admin").map((item) => item.key),
    ["overview", "realtime", "tasks", "devices", "connectors", "events", "algorithms", "agents", "assets", "settings"]
  );
});

test("member navigation hides management and ungranted agent capabilities", () => {
  assert.deepEqual(
    visibleProjectNavigation("member").map((item) => item.key),
    ["overview", "realtime", "tasks", "devices", "events", "assets"]
  );
  assert(visibleProjectNavigation("member", ["agent:use"]).some((item) => item.key === "agents"));
  assert(!visibleProjectNavigation("member").some((item) => item.key === "connectors"));
});

test("project overview is the stable project root and switch target", () => {
  assert.equal(projectNavigationHref(42, ""), "/projects/42");
  assert.equal(projectNavigationHref(42, "devices"), "/projects/42/devices");
});
