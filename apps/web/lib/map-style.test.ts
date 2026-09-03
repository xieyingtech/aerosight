import assert from "node:assert/strict";
import test from "node:test";

import { vectorStreetMapStyle } from "./map-style.ts";

test("MapLibre uses the OpenFreeMap vector street style", () => {
  assert.equal(vectorStreetMapStyle, "https://tiles.openfreemap.org/styles/liberty");
  assert.doesNotMatch(vectorStreetMapStyle, /tile\.openstreetmap\.org/);
});
