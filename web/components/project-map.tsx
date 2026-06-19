"use client";

import Map from "react-map-gl/maplibre";

export function ProjectMap() {
  return (
    <div className="h-[360px] overflow-hidden rounded-md border border-slate-200 bg-slate-100">
      <Map
        initialViewState={{ longitude: 116.397, latitude: 39.908, zoom: 10 }}
        mapStyle="https://demotiles.maplibre.org/style.json"
        style={{ width: "100%", height: "100%" }}
      />
    </div>
  );
}
