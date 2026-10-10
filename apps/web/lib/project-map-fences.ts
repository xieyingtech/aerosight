import type { createProjectMapModel } from "./project-map-model.ts";

// Decorative dimensions only; these are not measured fence heights.
export const fenceHeight = 80;
export const fenceBands = 16;

export function createProjectMapFences(model: ReturnType<typeof createProjectMapModel>) {
  const features: Array<{ type: "Feature"; properties: { kind: "wall" | "post" }; geometry: { type: "Polygon"; coordinates: number[][][] } }> = [];
  const add = (ring: number[][], kind: "wall" | "post") => {
    features.push({ type: "Feature", properties: { kind }, geometry: { type: "Polygon", coordinates: [ring] } });
  };
  for (const feature of model.features) {
    if (feature.properties.layerKind !== "region") continue;
    const geometry = feature.geometry;
    const polygons = geometry.type === "Polygon" ? [geometry.coordinates] : geometry.type === "MultiPolygon" ? geometry.coordinates : [];
    for (const rings of polygons) for (const ring of rings) {
      for (let i = 0; i < ring.length - 1 && features.length < 8192; i++) {
        const a = ring[i], b = ring[i + 1];
        if (![a[0], a[1], b[0], b[1]].every(Number.isFinite) || Math.abs(a[1]) > 85 || Math.abs(b[1]) > 85 || Math.abs(b[0] - a[0]) > 180) continue;
        const lonScale = 111320 * Math.cos((a[1] + b[1]) * Math.PI / 360);
        const dx = (b[0] - a[0]) * lonScale, dy = (b[1] - a[1]) * 111320;
        const length = Math.hypot(dx, dy);
        if (length < 0.01) continue;
        const ox = -dy / length * 0.65 / lonScale, oy = dx / length * 0.65 / 111320;
        add([[a[0] + ox, a[1] + oy], [b[0] + ox, b[1] + oy], [b[0] - ox, b[1] - oy], [a[0] - ox, a[1] - oy], [a[0] + ox, a[1] + oy]], "wall");
        const posts = Math.min(32, Math.max(1, Math.ceil(length / 45)));
        for (let p = 0; p < posts && features.length < 8192; p++) {
          const x = a[0] + (b[0] - a[0]) * p / posts, y = a[1] + (b[1] - a[1]) * p / posts;
          const w = 1.5 / lonScale, h = 1.5 / 111320;
          add([[x - w, y - h], [x + w, y - h], [x + w, y + h], [x - w, y + h], [x - w, y - h]], "post");
        }
      }
    }
  }
  return { type: "FeatureCollection" as const, features };
}
