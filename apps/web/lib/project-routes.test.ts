import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import test from "node:test";
import { projectNavigationHref, visibleProjectNavigation } from "./project-navigation.ts";

test("every manager project navigation entry has a routable page", () => {
  for (const item of visibleProjectNavigation("owner")) {
    const url = new URL(projectNavigationHref(42, item.segment), "http://frontend.test");
    assert.equal(url.searchParams.get("projectId"), "42");
    const page = `app/(app)${url.pathname}page.tsx`;
    assert(existsSync(page), `missing project route: ${page}`);
  }
});
