"use client";

import { Fragment, useMemo, useRef, useState, type CSSProperties } from "react";
import Link from "next/link";
import { AlertTriangleIcon, ArrowUpRightIcon, XIcon, CpuIcon, CameraIcon, ScanEyeIcon, FlagIcon, RouteIcon, WaypointsIcon, FenceIcon, GlobeIcon, MapIcon } from "lucide-react";
import { DeviceTypeIcon } from "@/components/device-type-icon";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { overviewSelectionHref } from "@/lib/overview-map-core";
import { createPortal } from "react-dom";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuCheckboxItem } from "@/components/ui/dropdown-menu";
import Map, { Layer, Marker, NavigationControl, Popup, Source, type MapRef } from "react-map-gl/maplibre";
import { cn } from "@/lib/utils";
import { vectorStreetMapStyle } from "@/lib/map-style";
import { createProjectMapFences, fenceBands, fenceHeight } from "@/lib/project-map-fences";
import { createProjectMapModel, projectMapDeviceTypes, filterProjectMapModelByDeviceTypes, filterProjectMapModelByLayers, filterProjectMapModelByTime, firstMapCoordinate, projectMapLayerCounts, projectMapLayers } from "@/lib/project-map-model";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import type { SituationSelection } from "@/lib/situation-state";

const interactiveLayers = ["regions-fill", "algorithm-results-fill", "mission-routes-line", "tracks-line"];
// Tracks follow their owning device layer and have no separate visibility control.
const selectableLayers = projectMapLayers.filter(layer => layer.id !== "tracks" && layer.id !== "devices");
const noExcludedLayers: readonly typeof projectMapLayers[number]["id"][] = [];
const mapIcons = {
  "region": FenceIcon, "mission-route": RouteIcon, "track": WaypointsIcon,
  "algorithm-results": ScanEyeIcon, "media": CameraIcon, "issue": FlagIcon,
  "device-generic": CpuIcon
};
const markerColors = {
  "region": "#0d9488", "mission-route": "#8b5cf6", "track": "#2563eb",
  "algorithm-results": "#f97316", "media": "#a855f7", "issue": "#ef4444",
  "device-generic": "#0284c7"
};

export function ProjectMap({ snapshot, className, controlsTarget, controlsClassName, compactControls = false, showPopups = false, showProjectionControl = false, selection, range, onSelect, excludedLayers = noExcludedLayers }: {
  snapshot: ProjectSituationSnapshot;
  className?: string;
  controlsTarget?: HTMLDivElement | null;
  compactControls?: boolean;
  controlsClassName?: string;
  showPopups?: boolean;
  showProjectionControl?: boolean;
  excludedLayers?: readonly typeof projectMapLayers[number]["id"][];
  selection?: SituationSelection | null;
  range?: { from: string; to: string } | null;
  onSelect?: (selection: SituationSelection) => void;
}) {
  const mapRef = useRef<MapRef>(null);
  const flatCamera = useRef<{ center: [number, number]; zoom: number; bearing: number; pitch: number } | null>(null);
  const [projection, setProjection] = useState<"mercator" | "globe">("mercator");
  const projectionMode = useRef<"mercator" | "globe">("mercator");
  const [mapReady, setMapReady] = useState(false);
  const syncBuildings = (mode = projectionMode.current) => {
    if (!showProjectionControl) return;
    const map = mapRef.current?.getMap();
    const visibility = mode === "globe" ? "visible" : "none";
    for (const layer of map?.getStyle()?.layers ?? []) {
      if (layer.type === "fill-extrusion" && "source-layer" in layer && layer["source-layer"] === "building" && map?.getLayoutProperty(layer.id, "visibility") !== visibility) {
        map?.setLayoutProperty(layer.id, "visibility", visibility);
      }
    }
  };
  const changeProjection = (next: "mercator" | "globe") => {
    const map = mapRef.current;
    if (!map || !mapReady || next === projection) return;
    map.stop();
    const position = map.getCenter();
    if (next === "globe") {
      flatCamera.current = { center: [position.lng, position.lat], zoom: map.getZoom(), bearing: map.getBearing(), pitch: map.getPitch() };
    }
    const camera = next === "globe"
      ? { center: [position.lng, position.lat] as [number, number], zoom: map.getZoom(), bearing: map.getBearing(), pitch: 55 }
      : flatCamera.current;
    // Apply before moving the camera; the prop also retains projection on style reload.
    projectionMode.current = next;
    map.getMap().setProjection({ type: next });
    setProjection(next);
    syncBuildings(next);
    if (camera) {
      if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) map.jumpTo(camera);
      else map.flyTo({ ...camera, duration: 1400 });
    }
  };
  const model = useMemo(() => filterProjectMapModelByLayers(
    filterProjectMapModelByTime(createProjectMapModel(snapshot), range ?? null),
    new Set(projectMapLayers.filter(layer => !excludedLayers.includes(layer.id)).map(layer => layer.id))
  ), [snapshot, range, excludedLayers]);
  const controlsLayers = selectableLayers.filter(layer => !excludedLayers.includes(layer.id));
  const [popup, setPopup] = useState<{ longitude: number; latitude: number; selection: SituationSelection } | null>(null);
  const counts = projectMapLayerCounts(model);
  const [visible, setVisible] = useState(() => new Set(projectMapLayers.map((layer) => layer.id)));
  const [hiddenTypes, setHiddenTypes] = useState<Set<string>>(() => new Set());
  const deviceTypes = projectMapDeviceTypes(model);
  const visibleModel = filterProjectMapModelByDeviceTypes(filterProjectMapModelByLayers(model, visible), hiddenTypes);
  const fences = useMemo(() => createProjectMapFences(model), [model]);
  const popupFeature = popup && visibleModel.features.find(item => item.properties.entityId === popup.selection.entityId && item.properties.layerKind === popup.selection.lane);
  const popupLayer = popupFeature && projectMapLayers.find(layer => layer.kind === popupFeature.properties.layerKind);
  const hiddenOwnerTypes = [...hiddenTypes];
  const activeSelection = showPopups ? popup?.selection : selection;
  const popupHref = popup && overviewSelectionHref(snapshot.project.id, popup.selection);
  const center = firstMapCoordinate(model) ?? [116.397, 39.908];
  const toggle = (id: typeof projectMapLayers[number]["id"]) => {
    if (visible.has(id) && popupLayer?.id === id) setPopup(null);
    setVisible(current => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };
  const toggleType = (key: string) => {
    if (!hiddenTypes.has(key) && (popupFeature?.properties.deviceTypeKey === key || popupFeature?.properties.ownerDeviceTypeKey === key)) setPopup(null);
    setHiddenTypes(current => { const next = new Set(current); if (next.has(key)) next.delete(key); else next.add(key); return next; });
  };
  const badgeClass = (active: boolean) => cn("h-7 border px-2.5 shadow-sm", active ? "border-primary/25 bg-background/95" : "bg-muted/95 text-muted-foreground");
  const projectionControls = showProjectionControl && <div role="group" aria-label="地图视角" className="inline-flex gap-1 rounded-lg border bg-background/95 p-1 shadow-sm">
      <Button type="button" size="sm" variant={projection === "mercator" ? "secondary" : "ghost"} className="h-7 px-2.5" disabled={!mapReady} aria-pressed={projection === "mercator"} onClick={() => changeProjection("mercator")}><MapIcon aria-hidden="true" className="size-3.5" />平面</Button>
      <Button type="button" size="sm" variant={projection === "globe" ? "secondary" : "ghost"} className="h-7 px-2.5" disabled={!mapReady} aria-pressed={projection === "globe"} onClick={() => changeProjection("globe")}><GlobeIcon aria-hidden="true" className="size-3.5" />3D</Button>
    </div>;
  const controls = <div className="space-y-2" aria-label="地图图层">
    <div className="flex flex-wrap gap-1.5" aria-label="地图要素">
      {controlsLayers.map(layer => { const Icon = mapIcons[layer.kind]; return <Badge key={layer.id} asChild variant={visible.has(layer.id) ? "secondary" : "outline"} className={badgeClass(visible.has(layer.id))}><button type="button" aria-pressed={visible.has(layer.id)} onClick={() => toggle(layer.id)}><Icon aria-hidden="true" className="mr-1 size-3.5" />{layer.label}<span className="ml-1 tabular-nums">{counts[layer.id]}</span></button></Badge>; })}
    </div>
    <div className="flex flex-wrap gap-1.5" aria-label="设备类型">
      {deviceTypes.map(type => <Badge key={type.key} asChild variant={!hiddenTypes.has(type.key) ? "secondary" : "outline"} className={badgeClass(!hiddenTypes.has(type.key))}><button type="button" aria-pressed={!hiddenTypes.has(type.key)} onClick={() => toggleType(type.key)}><DeviceTypeIcon name={type.icon} className="mr-1 size-3.5" />{type.label}<span className="ml-1 tabular-nums">{type.count}</span></button></Badge>)}
    </div>
  </div>;
  return (
    <div className={cn("relative h-[520px] overflow-hidden rounded-xl border bg-muted", className)}>
      {projectionControls && <div className="absolute right-3 top-3 z-10">{projectionControls}</div>}
      {controlsTarget !== undefined ? controlsTarget && createPortal(controls, controlsTarget) : <div className={cn("absolute left-3 right-12 top-3 z-10", controlsClassName)}>
        {compactControls ? <DropdownMenu><DropdownMenuTrigger asChild><Button size="sm" variant="outline">地图图层</Button></DropdownMenuTrigger><DropdownMenuContent>{controlsLayers.map(layer => <DropdownMenuCheckboxItem key={layer.id} checked={visible.has(layer.id)} onCheckedChange={() => toggle(layer.id)} onSelect={event => event.preventDefault()}>{layer.label}<span className="ml-auto pl-3 tabular-nums">{counts[layer.id]}</span></DropdownMenuCheckboxItem>)}{deviceTypes.map(type => <DropdownMenuCheckboxItem key={type.key} checked={!hiddenTypes.has(type.key)} onCheckedChange={() => toggleType(type.key)} onSelect={event => event.preventDefault()}><DeviceTypeIcon name={type.icon} className="mr-1 size-3.5" />{type.label}<span className="ml-auto pl-3 tabular-nums">{type.count}</span></DropdownMenuCheckboxItem>)}</DropdownMenuContent></DropdownMenu> : controls}
      </div>}
      <Map
        ref={mapRef}
        projection={{ type: projection }}
        onLoad={() => { syncBuildings(); setMapReady(true); }}
        onStyleData={() => syncBuildings()}
        attributionControl={{ compact: true }}
        initialViewState={{ longitude: center[0], latitude: center[1], zoom: model.features.length ? 13 : 5 }}
        interactiveLayerIds={interactiveLayers}
        mapStyle={vectorStreetMapStyle}
        onClick={(event) => {
          const properties = event.features?.[0]?.properties;
          if (!properties?.entityId) { setPopup(null); return; }
          const selected = {
            lane: String(properties.layerKind), entityId: String(properties.entityId),
            label: String(properties.label ?? "地图要素"), timestamp: properties.capturedAt ? String(properties.capturedAt) : undefined
          };
          onSelect?.(selected);
          if (showPopups) {
            const feature = event.features?.[0];
            const position = feature?.geometry.type === "Point" ? feature.geometry.coordinates : [event.lngLat.lng, event.lngLat.lat];
            setPopup({ longitude: position[0], latitude: position[1], selection: selected });
          }
        }}
        style={{ width: "100%", height: "100%" }}
      >
        <NavigationControl position="bottom-right" />
        <Source data={model} id="project-situation" type="geojson">

          {visible.has("regions") && <Layer id="regions-fill" type="fill" filter={["==", ["get", "layerKind"], "region"]} paint={{ "fill-color": "#14b8a6", "fill-opacity": 0.14, "fill-outline-color": "#0f766e" }} />}
          {visible.has("algorithm-results") && <Layer id="algorithm-results-fill" type="fill" filter={["==", ["get", "layerKind"], "algorithm-results"]} paint={{ "fill-color": "#f97316", "fill-opacity": 0.38, "fill-outline-color": "#c2410c" }} />}
          {visible.has("mission-routes") && <Layer id="mission-routes-line" type="line" filter={["all", ["==", ["get", "layerKind"], "mission-route"], ["!", ["in", ["coalesce", ["get", "ownerDeviceTypeKey"], ""], ["literal", hiddenOwnerTypes]]]]} paint={{ "line-color": "#8b5cf6", "line-dasharray": [2, 1.5], "line-width": 3 }} />}
          {visible.has("tracks") && <Layer id="tracks-line" type="line" filter={["all", ["==", ["get", "layerKind"], "track"], ["!", ["in", ["coalesce", ["get", "ownerDeviceTypeKey"], ""], ["literal", hiddenOwnerTypes]]]]} paint={{ "line-color": "#2563eb", "line-opacity": 0.8, "line-width": 3 }} />}
        </Source>
        {projection === "globe" && visible.has("regions") && <Source id="project-region-fences" type="geojson" data={fences}>
          {Array.from({ length: fenceBands }, (_, band) => <Fragment key={band}><Layer source="project-region-fences" id={`region-fence-${band}`} type="fill-extrusion" filter={["==", ["get", "kind"], "wall"]} paint={{
            "fill-extrusion-color": "#14b8a6",
            "fill-extrusion-base": band * fenceHeight / fenceBands,
            "fill-extrusion-height": (band + 1) * fenceHeight / fenceBands,
            "fill-extrusion-opacity": 0.58 * Math.pow(1 - band / (fenceBands - 1), 1.5),
            "fill-extrusion-vertical-gradient": false
          }} /><Layer source="project-region-fences" id={`region-fence-post-${band}`} type="fill-extrusion" filter={["==", ["get", "kind"], "post"]} paint={{
            "fill-extrusion-color": "#0d9488",
            "fill-extrusion-base": band * fenceHeight / fenceBands,
            "fill-extrusion-height": (band + 1) * fenceHeight / fenceBands,
            "fill-extrusion-opacity": 0.95 * Math.pow(1 - band / (fenceBands - 1), 0.8),
            "fill-extrusion-vertical-gradient": false
          }} /></Fragment>)}
        </Source>}
        {visibleModel.features.filter(item => item.geometry.type === "Point").map(item => {
          if (item.geometry.type !== "Point") return null;
          const coordinates = item.geometry.coordinates;
          const props = item.properties;
          const Icon = mapIcons[props.layerKind];
          const selected = activeSelection?.lane === props.layerKind && activeSelection.entityId === props.entityId;
          const haloColor = props.warningSeverity === "error" ? "#ef4444" : "#f59e0b";
          const color = props.layerKind.startsWith("device-") && props.status === "offline" ? "#94a3b8"
            : props.warningSeverity ? haloColor : markerColors[props.layerKind];
          return <Marker key={`${props.layerKind}-${props.entityId}`} longitude={coordinates[0]} latitude={coordinates[1]} anchor="center">
            <button type="button" aria-label={props.label} aria-pressed={selected} title={props.label}
              className={cn("project-map-marker", selected && "project-map-marker-selected")}
              style={{ "--marker-color": color, "--marker-halo-color": haloColor } as CSSProperties}
              onClick={event => {
                event.stopPropagation();
                const selection = { lane: props.layerKind, entityId: props.entityId, label: props.label, timestamp: props.capturedAt };
                onSelect?.(selection);
                if (showPopups) setPopup({ longitude: coordinates[0], latitude: coordinates[1], selection });
              }}>
              {props.warningSeverity && <span aria-hidden="true" className="project-map-marker-halo"><span /><span /></span>}
              <span className="project-map-marker-core">{props.layerKind === "device-generic" ? <DeviceTypeIcon name={props.deviceTypeIcon} size={16} /> : <Icon aria-hidden="true" size={16} strokeWidth={2} />}</span>
            </button>
          </Marker>;
        })}
        {showPopups && popup && popupFeature && popupLayer && visible.has(popupLayer.id) && <Popup key={`${popup.selection.lane}-${popup.selection.entityId}`} longitude={popup.longitude} latitude={popup.latitude} offset={20} closeOnClick={false} onClose={() => setPopup(null)} maxWidth="300px" closeButton={false} className="project-map-popup">
          <Card size="sm" className="w-64 gap-3 rounded-xl ring-0">
            <CardHeader className="relative pr-10">
              <div className="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground">{popupFeature.properties.layerKind === "device-generic" ? <DeviceTypeIcon name={popupFeature.properties.deviceTypeIcon} className="size-3.5" /> : <MapFeatureIcon kind={popupLayer.kind} />}{popupFeature.properties.deviceTypeName ?? popupLayer.label}</div>
              <CardTitle className="text-sm leading-relaxed">{popupFeature.properties.label}</CardTitle>
              <Button className="absolute right-2 top-0 size-7 text-muted-foreground" variant="ghost" size="icon-sm" aria-label="关闭标点详情" onClick={() => setPopup(null)}><XIcon className="size-3.5" /></Button>
            </CardHeader>
            <CardContent className="space-y-3">
              {popupFeature.properties.status && <div className="flex items-center gap-2 text-xs"><span className={cn("size-1.5 rounded-full", popupFeature.properties.status === "online" ? "bg-emerald-500" : "bg-muted-foreground")} /><span>{({ online: "在线", offline: "离线", open: "待处理", running: "执行中", degraded: "异常", active: "活动中" } as Record<string, string>)[popupFeature.properties.status] ?? popupFeature.properties.status}</span></div>}
              {popupFeature.properties.warningSeverity && <div className="flex items-start gap-2.5 rounded-lg bg-muted/50 p-2.5"><AlertTriangleIcon className={cn("mt-0.5 size-4 shrink-0", popupFeature.properties.warningSeverity === "error" ? "text-red-500" : "text-amber-500")} /><div className="space-y-1"><p className="text-xs font-medium">{popupFeature.properties.warningSeverity === "error" ? "设备异常" : "需要关注"}</p><p className="whitespace-pre-line text-xs leading-relaxed text-muted-foreground">{popupFeature.properties.warningMessage}</p></div></div>}
              {popupFeature.properties.capturedAt && <p className="text-[11px] text-muted-foreground">更新于 {new Date(popupFeature.properties.capturedAt).toLocaleString("zh-CN")}</p>}
            </CardContent>
            {popupHref && <CardFooter className="bg-transparent py-2.5"><Button asChild size="sm" variant="secondary" className="w-full justify-between"><Link href={popupHref.href}>{popupHref.label}<ArrowUpRightIcon className="size-3.5" /></Link></Button></CardFooter>}
          </Card>
        </Popup>}
      </Map>
    </div>
  );
}

function MapFeatureIcon({ kind }: { kind: keyof typeof mapIcons }) {
  const Icon = mapIcons[kind];
  return <Icon aria-hidden="true" className="size-3.5" />;
}
