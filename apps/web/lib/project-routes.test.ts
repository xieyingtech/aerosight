import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import test from "node:test";
import { visibleProjectNavigation } from "./project-navigation.ts";

test("every manager project navigation entry has a routable page", () => {
  for (const item of visibleProjectNavigation("owner")) {
    const suffix = item.segment ? `/${item.segment}` : "";
    const page = `app/(app)/projects/[id]${suffix}/page.tsx`;
    assert(existsSync(page), `missing project route: ${page}`);
  }
});
