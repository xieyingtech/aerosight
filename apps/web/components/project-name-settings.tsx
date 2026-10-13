"use client";

import { APIStateView } from "@/components/api-state";
import { NameSettingsForm } from "@/components/name-settings-form";
import { useAPI } from "@/lib/use-api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function ProjectNameSettings({ projectId }: { projectId: number }) {
  const state = useAPI<{ name: string }>(`/api/projects/${projectId}`);
  return <APIStateView state={state}>{project => <Card><CardHeader><CardTitle>项目基本信息</CardTitle></CardHeader><CardContent><div className="flex flex-wrap items-center justify-between gap-4"><div className="min-w-0"><p className="text-sm text-muted-foreground">项目名称</p><p className="mt-1 break-words font-medium">{project.name}</p></div><NameSettingsForm key={projectId} projectId={projectId} name={project.name} /></div></CardContent></Card>}</APIStateView>;
}
