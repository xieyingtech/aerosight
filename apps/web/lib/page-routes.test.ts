import assert from "node:assert/strict";
import test from "node:test";
import { canonicalPageHref, projectPageHref, resourcePages, scopedPageQuery } from "./page-routes.ts";

test("path identity overrides stale query scope and preserves filters", () => {
  const query = scopedPageQuery("/projects/7/tasks/12/", "projectId=9&taskId=99&status=open&layer=a&layer=b");
  assert.equal(query.get("projectId"), "7");
  assert.equal(query.get("taskId"), "12");
  assert.deepEqual(query.getAll("layer"), ["a", "b"]);
  assert.equal(query.get("status"), "open");
});
test("resource page paths round trip without identity query parameters", () => {
  for (const [page, resource] of Object.entries(resourcePages)) {
    const id = ["taskId", "issueId"].includes(resource.key) || page === "tasks/runs/detail" || page === "inspection/summary" ? "12" : "01234567-89ab-cdef-0123-456789abcdef";
    const href = projectPageHref(7, page, {[resource.key]: id, filter: "a&b"});
    const url = new URL(href, "http://localhost");
    assert(!url.searchParams.has("projectId"));
    assert(!url.searchParams.has(resource.key));
    const query = scopedPageQuery(url.pathname, url.search);
    assert.equal(query.get(resource.key), id);
    assert.equal(query.get("projectId"), "7");
    assert.equal(query.get("filter"), "a&b");
  }
});
test("realtime device identity is in the path and stream selection stays in query", () => {
  const href = canonicalPageHref("/projects/realtime/?projectId=7&deviceId=12&streamId=21");
  assert.equal(href, "/projects/7/realtime/devices/12/?streamId=21");
  assert.equal(scopedPageQuery("/projects/7/realtime/devices/12/", "streamId=21").get("deviceId"), "12");
});
