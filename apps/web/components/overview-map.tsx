"use client";

import { CanvasWorkspace } from "@/components/canvas-workspace";
import { ProjectMap } from "@/components/project-map";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";

const overviewExcludedLayers = ["media", "algorithm-results"] as const;

export function OverviewMap({ snapshot }: { snapshot: ProjectSituationSnapshot }) {
  return <CanvasWorkspace title="项目地图" showTitle={false}>
    <ProjectMap className="h-full min-h-0 rounded-none border-0" controlsClassName="right-16" excludedLayers={overviewExcludedLayers} showPopups snapshot={snapshot} />
  </CanvasWorkspace>;
}
