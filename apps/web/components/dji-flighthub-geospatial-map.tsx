"use client";

import { useMemo, useState } from "react";
import Map, { Layer, NavigationControl, Source } from "react-map-gl/maplibre";

import type { FlightHubGeospatialGeometry, FlightHubGeospatialWorkspace } from "@/lib/dji-flighthub-geospatial-core";
import { vectorStreetMapStyle } from "@/lib/map-style";
import { cn } from "@/lib/utils";

type LayerKind = "map-element" | "flight-area" | "air-sense-warning";
type MapFeature = {
  type: "Feature";
  geometry: FlightHubGeospatialGeometry;
  properties: { id: string; kind: LayerKind; label: string; freshness: string; version: string };
};

const layers: Array<{ id: LayerKind; label: string }> = [
  { id: "map-element", label: "地图标注" },
  { id: "flight-area", label: "飞行区" },
  { id: "air-sense-warning", label: "空间告警" },
];

function firstCoordinate(geometry: FlightHubGeospatialGeometry): [number, number] | null {
  let current: unknown = geometry.coordinates;
  while (Array.isArray(current) && Array.isArray(current[0])) current = current[0];
  if (!Array.isArray(current) || !Number.isFinite(Number(current[0])) || !Number.isFinite(Number(current[1]))) return null;
  return [Number(current[0]), Number(current[1])];
}

export function FlightHubGeospatialMap({ workspace, className }: {
  workspace: FlightHubGeospatialWorkspace;
  className?: string;
}) {
  const model = useMemo(() => {
    const features: MapFeature[] = [];
    for (const item of workspace.mapElements) if (item.geometry) features.push({
      type: "Feature", geometry: item.geometry,
      properties: { id: item.id, kind: "map-element", label: item.name, freshness: item.freshness, version: item.versionFingerprint ?? "—" },
    });
    for (const item of workspace.flightAreas) if (item.geometry) features.push({
      type: "Feature", geometry: item.geometry,
      properties: { id: item.id, kind: "flight-area", label: item.name, freshness: item.freshness, version: item.versionFingerprint ?? "—" },
    });
    for (const item of workspace.airSenseWarnings) if (item.geometry) features.push({
      type: "Feature", geometry: item.geometry,
      properties: { id: item.id, kind: "air-sense-warning", label: item.name, freshness: item.freshness, version: item.versionFingerprint ?? "—" },
    });
    return { type: "FeatureCollection" as const, features };
  }, [workspace]);
  const center = model.features.map((item) => firstCoordinate(item.geometry)).find((item): item is [number, number] => item !== null) ?? [116.397, 39.908];
  const [visible, setVisible] = useState(() => new Set<LayerKind>(layers.map((layer) => layer.id)));
  const [selected, setSelected] = useState<MapFeature["properties"] | null>(null);
  const toggle = (id: LayerKind) => setVisible((current) => {
    const next = new Set(current);
    if (next.has(id)) next.delete(id); else next.add(id);
    return next;
  });
  return <div className={cn("relative h-[500px] overflow-hidden rounded-xl border bg-muted", className)}>
    <div className="absolute left-3 right-12 top-3 z-10 flex flex-wrap gap-1.5">
      {layers.map((layer) => <button
        aria-pressed={visible.has(layer.id)}
        className={cn("rounded-full border px-2.5 py-1 text-xs shadow-sm backdrop-blur transition-colors",
          visible.has(layer.id) ? "border-primary/30 bg-background/95 text-foreground" : "border-border/60 bg-muted/80 text-muted-foreground")}
        key={layer.id}
        onClick={() => toggle(layer.id)}
        type="button"
      >{layer.label}</button>)}
    </div>
    <Map
      initialViewState={{ longitude: center[0], latitude: center[1], zoom: model.features.length ? 13 : 5 }}
      interactiveLayerIds={["geospatial-map-points", "geospatial-flight-points", "geospatial-air-sense-points", "geospatial-map-fill", "geospatial-flight-fill"]}
      mapStyle={vectorStreetMapStyle}
      onClick={(event) => {
        const properties = event.features?.[0]?.properties;
        setSelected(properties?.id ? {
          id: String(properties.id), kind: String(properties.kind) as LayerKind, label: String(properties.label ?? "空间要素"),
          freshness: String(properties.freshness ?? "unknown"), version: String(properties.version ?? "—"),
        } : null);
      }}
      style={{ width: "100%", height: "100%" }}
    >
      <NavigationControl position="bottom-right" />
      <Source data={model} id="flighthub-geospatial" type="geojson">
        {visible.has("map-element") && <Layer id="geospatial-map-fill" type="fill" filter={["all", ["==", ["get", "kind"], "map-element"], ["==", ["geometry-type"], "Polygon"]]} paint={{ "fill-color": "#8b5cf6", "fill-opacity": 0.22, "fill-outline-color": "#6d28d9" }} />}
        {visible.has("map-element") && <Layer id="geospatial-map-lines" type="line" filter={["==", ["get", "kind"], "map-element"]} paint={{ "line-color": "#7c3aed", "line-width": 3 }} />}
        {visible.has("map-element") && <Layer id="geospatial-map-points" type="circle" filter={["all", ["==", ["get", "kind"], "map-element"], ["==", ["geometry-type"], "Point"]]} paint={{ "circle-color": "#8b5cf6", "circle-radius": 7, "circle-stroke-color": "#fff", "circle-stroke-width": 2 }} />}
        {visible.has("flight-area") && <Layer id="geospatial-flight-fill" type="fill" filter={["all", ["==", ["get", "kind"], "flight-area"], ["==", ["geometry-type"], "Polygon"]]} paint={{ "fill-color": "#14b8a6", "fill-opacity": 0.18, "fill-outline-color": "#0f766e" }} />}
        {visible.has("flight-area") && <Layer id="geospatial-flight-lines" type="line" filter={["==", ["get", "kind"], "flight-area"]} paint={{ "line-color": "#0f766e", "line-dasharray": [2, 1.5], "line-width": 3 }} />}
        {visible.has("flight-area") && <Layer id="geospatial-flight-points" type="circle" filter={["all", ["==", ["get", "kind"], "flight-area"], ["==", ["geometry-type"], "Point"]]} paint={{ "circle-color": "#14b8a6", "circle-radius": 8, "circle-stroke-color": "#fff", "circle-stroke-width": 2 }} />}
        {visible.has("air-sense-warning") && <Layer id="geospatial-air-sense-points" type="circle" filter={["==", ["get", "kind"], "air-sense-warning"]} paint={{ "circle-color": ["match", ["get", "freshness"], "fresh", "#ef4444", "#64748b"], "circle-radius": 9, "circle-stroke-color": "#fff", "circle-stroke-width": 2.5 }} />}
      </Source>
    </Map>
    <div className="absolute bottom-3 left-3 max-w-[calc(100%-5rem)] rounded-md border bg-background/90 px-2.5 py-1.5 text-xs shadow-sm backdrop-blur">
      {selected ? <><span className="font-medium">{selected.label}</span> · {selected.freshness} · {selected.version}</> : <>{model.features.length} 个可绘制空间要素 · 坐标待验收数据仅供观察</>}
    </div>
  </div>;
}
