import type { StyleSpecification } from "maplibre-gl";

export const streetMapStyle: StyleSpecification = {
  version: 8,
  name: "OpenStreetMap streets",
  sources: {
    openstreetmap: {
      type: "raster",
      tiles: ["https://tile.openstreetmap.org/{z}/{x}/{y}.png"],
      tileSize: 256,
      maxzoom: 19,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    },
  },
  layers: [
    {
      id: "street-background",
      type: "background",
      paint: { "background-color": "#e8edf2" },
    },
    {
      id: "street-map",
      type: "raster",
      source: "openstreetmap",
      minzoom: 0,
      maxzoom: 20,
    },
  ],
};
