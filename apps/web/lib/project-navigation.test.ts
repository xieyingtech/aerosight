import assert from "node:assert/strict";
import test from "node:test";
import { legacyProjectEventListHref, projectNavigationHref, visibleProjectNavigation } from "./project-navigation.ts";

test("project manager sees the complete project workspace navigation", () => {
  assert.deepEqual(
    visibleProjectNavigation("admin").map((item) => item.key),
    ["overview", "realtime", "tasks", "devices", "connectors", "issues", "algorithms", "agents", "assets", "settings"]
  );
});

test("member navigation hides management and ungranted agent capabilities", () => {
  assert.deepEqual(
    visibleProjectNavigation("member").map((item) => item.key),
    ["overview", "realtime", "tasks", "devices", "issues", "assets"]
  );
  assert(visibleProjectNavigation("member", ["agent:use"]).some((item) => item.key === "agents"));
  assert(!visibleProjectNavigation("member").some((item) => item.key === "connectors"));
});

test("project overview is the stable project root and switch target", () => {
  assert.equal(projectNavigationHref(42, ""), "/projects/detail/?projectId=42");
  assert.equal(projectNavigationHref(42, "devices"), "/projects/devices/?projectId=42");
});

test("resource links encode values and retain their authoritative project", () => {
  const url = new URL(projectNavigationHref(42, "issues/detail", {projectId:99, issueId:7, selected:"a&b 中文"}), "http://frontend.test");
  assert.equal(url.pathname, "/projects/issues/detail/");
  assert.equal(url.searchParams.get("projectId"), "42");
  assert.equal(url.searchParams.get("issueId"), "7");
  assert.equal(url.searchParams.get("selected"), "a&b 中文");
});

test("legacy alert list links migrate to the project issue list", () => {
  assert.equal(legacyProjectEventListHref(42), "/projects/issues/?projectId=42");
  assert(!visibleProjectNavigation("admin").some((item) => item.segment === "events"));
});

test("connector-specific workspaces do not become global project navigation", () => {
  const items = visibleProjectNavigation("admin");
  for (const segment of ["flight-operations", "geospatial", "models"]) {
    assert(!items.some((item) => item.segment === segment));
  }
});
