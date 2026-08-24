"use client";

import { useMemo, useState } from "react";
import Map, { Layer, NavigationControl, Source } from "react-map-gl/maplibre";
import { cn } from "@/lib/utils";
import { createProjectMapModel, firstMapCoordinate, projectMapLayers } from "@/lib/project-map-model";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";

export function ProjectMap({ snapshot, className }: { snapshot: ProjectSituationSnapshot; className?: string }) {
  const model = useMemo(() => createProjectMapModel(snapshot), [snapshot]);
  const center = firstMapCoordinate(model) ?? [116.397, 39.908];
  const [visible, setVisible] = useState(() => new Set(projectMapLayers.map((layer) => layer.id)));
  const toggle = (id: typeof projectMapLayers[number]["id"]) => setVisible((current) => {
    const next = new Set(current);
    if (next.has(id)) next.delete(id); else next.add(id);
    return next;
  });
  return (
    <div className={cn("relative h-[520px] overflow-hidden rounded-xl border bg-muted", className)}>
      <div className="absolute left-3 right-12 top-3 z-10 flex flex-wrap gap-1.5">
        {projectMapLayers.map((layer) => (
          <button aria-pressed={visible.has(layer.id)} className={cn(
            "rounded-full border px-2.5 py-1 text-xs shadow-sm backdrop-blur transition-colors",
            visible.has(layer.id) ? "border-primary/30 bg-background/95 text-foreground" : "border-border/60 bg-muted/80 text-muted-foreground"
          )} key={layer.id} onClick={() => toggle(layer.id)} type="button">{layer.label}</button>
        ))}
      </div>
      <Map
        initialViewState={{ longitude: center[0], latitude: center[1], zoom: model.features.length ? 13 : 5 }}
        mapStyle="https://demotiles.maplibre.org/style.json"
        style={{ width: "100%", height: "100%" }}
      >
        <NavigationControl position="bottom-right" />
        <Source data={model} id="project-situation" type="geojson">
          {visible.has("regions") && <Layer id="regions-fill" type="fill" filter={["==", ["get", "layerKind"], "region"]} paint={{ "fill-color": "#14b8a6", "fill-opacity": 0.14, "fill-outline-color": "#0f766e" }} />}
          {visible.has("suspected-construction") && <Layer id="suspected-fill" type="fill" filter={["==", ["get", "layerKind"], "suspected-construction"]} paint={{ "fill-color": "#f97316", "fill-opacity": 0.38, "fill-outline-color": "#c2410c" }} />}
          {visible.has("mission-routes") && <Layer id="mission-routes-line" type="line" filter={["==", ["get", "layerKind"], "mission-route"]} paint={{ "line-color": "#8b5cf6", "line-dasharray": [2, 1.5], "line-width": 3 }} />}
          {visible.has("tracks") && <Layer id="tracks-line" type="line" filter={["==", ["get", "layerKind"], "track"]} paint={{ "line-color": "#2563eb", "line-opacity": 0.8, "line-width": 3 }} />}
          {visible.has("media") && <Layer id="media-points" type="circle" filter={["==", ["get", "layerKind"], "media"]} paint={{ "circle-color": "#a855f7", "circle-radius": 5, "circle-stroke-color": "#fff", "circle-stroke-width": 2 }} />}
          {visible.has("alerts") && <Layer id="alert-points" type="circle" filter={["==", ["get", "layerKind"], "alert"]} paint={{ "circle-color": "#ef4444", "circle-radius": 8, "circle-stroke-color": "#fff", "circle-stroke-width": 2 }} />}
          {visible.has("suspected-construction") && <Layer id="suspected-points" type="circle" filter={["all", ["==", ["get", "layerKind"], "suspected-construction"], ["==", ["geometry-type"], "Point"]]} paint={{ "circle-color": "#f97316", "circle-radius": 7, "circle-stroke-color": "#fff", "circle-stroke-width": 2 }} />}
          {visible.has("drones") && <Layer id="device-drones" type="circle" filter={["==", ["get", "layerKind"], "device-drone"]} paint={{ "circle-color": "#0ea5e9", "circle-radius": 8, "circle-stroke-color": "#fff", "circle-stroke-width": 2.5 }} />}
          {visible.has("docks") && <Layer id="device-docks" type="circle" filter={["==", ["get", "layerKind"], "device-dock"]} paint={{ "circle-color": "#334155", "circle-radius": 7, "circle-stroke-color": "#fff", "circle-stroke-width": 2 }} />}
          {visible.has("ground-robots") && <Layer id="device-ground" type="circle" filter={["==", ["get", "layerKind"], "device-ground"]} paint={{ "circle-color": "#22c55e", "circle-radius": 8, "circle-stroke-color": "#fff", "circle-stroke-width": 2.5 }} />}
        </Source>
      </Map>
      <div className="absolute bottom-3 left-3 rounded-md border bg-background/90 px-2.5 py-1.5 text-xs shadow-sm backdrop-blur">
        {snapshot.freshness.isRealtime ? "实时数据" : "历史/等待数据"} · {model.features.length} 个地图要素
      </div>
    </div>
  );
}
