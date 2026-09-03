import assert from "node:assert/strict";
import test from "node:test";

import { streetMapStyle } from "./map-style.ts";

test("MapLibre uses an attributed OpenStreetMap street basemap", () => {
  const source = streetMapStyle.sources.openstreetmap;

  assert.equal(source.type, "raster");
  assert.deepEqual(source.tiles, ["https://tile.openstreetmap.org/{z}/{x}/{y}.png"]);
  assert.match(source.attribution ?? "", /OpenStreetMap/);
  assert(streetMapStyle.layers.some((layer) => layer.id === "street-map" && "source" in layer && layer.source === "openstreetmap"));
});
