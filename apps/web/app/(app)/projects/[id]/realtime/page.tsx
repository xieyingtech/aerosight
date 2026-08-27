import Link from "next/link";
import { Page } from "@/components/page";
import { getProject, requireUser } from "@/lib/data";
import { readProjectSituationSnapshot } from "@/lib/project-snapshot";
import { SituationExplorer } from "@/components/situation-explorer";
import { Badge } from "@/components/ui/badge";

export default async function RealtimeOperationsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const [project, user] = await Promise.all([getProject(projectId), requireUser()]);
  const snapshot = await readProjectSituationSnapshot(user.id, projectId);
  if (!snapshot) throw new Error("PROJECT_NOT_FOUND");
  return (
    <Page description={`${project.name} 的在线设备、直播与任务控制`} title="实时作业">
      <div className="space-y-4">
        <SituationExplorer mapClassName="h-[620px]" snapshot={snapshot} />
        <section className="rounded-xl border bg-card p-4"><div className="mb-3 flex items-center justify-between"><h2 className="font-medium">活动任务</h2><span className="text-xs text-muted-foreground">{project.permissions.includes("mission:operate") || project.role !== "member" ? "可控制" : "只读"}</span></div>
          <div className="grid gap-2 md:grid-cols-2">{snapshot.activeTasks.length ? snapshot.activeTasks.map((run) => <Link className="flex items-center justify-between rounded-lg border px-3 py-2 hover:bg-muted/50" href={`/projects/${projectId}/tasks/runs/${String(run.id)}`} key={String(run.id)}><span>{String(run.taskName)}</span><Badge variant="outline">{String(run.status)}</Badge></Link>) : <p className="text-sm text-muted-foreground">暂无活动任务</p>}</div>
        </section>
      </div>
    </Page>
  );
}
