"use client";

import Link from "next/link";
import { Plus } from "lucide-react";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export default function ProjectsPage() {
  return <Suspense fallback={<p role="status">正在加载…</p>}><ProjectsContent /></Suspense>;
}

function ProjectsContent() {
  const params = useSearchParams();
  const search = params.get("search") ?? "";
  const state = useAPI<Record<string, unknown>[]>(`/api/projects?search=${encodeURIComponent(search)}`);
  return <APIStateView state={state}>{(projects) => <ProjectsView projects={projects} params={{ search }} />}</APIStateView>;
}

function ProjectsView({ projects, params }: { projects: Record<string, unknown>[]; params: { search: string } }) {

  return (
    <Page
      actions={
        <Button asChild size="lg">
          <Link href="/projects/new"><Plus />新建项目</Link>
        </Button>
      }
      description={`共 ${projects.length} 个项目`}
      title="项目"
    >
      <form action="/projects" className="flex gap-2">
        <Input
          className="min-w-0 flex-1"
          defaultValue={params.search ?? ""}
          name="search"
          placeholder="按项目、团队名称搜索"
        />
        <Button type="submit" variant="outline">搜索</Button>
      </form>
      <DataTable
        columns={[
          {
            key: "name",
            label: "项目",
            render: (item) => (
              <div>
                <Link className="font-medium text-primary hover:underline" href={`/projects/detail/?projectId=${String(item.id)}`}>
                  <span className="text-muted-foreground">{String(item.teamName)}/</span>
                  {String(item.name)}
                </Link>
                <p className="mt-1 text-xs text-muted-foreground">{String(item.description ?? "暂无描述")}</p>
              </div>
            )
          },
          { key: "role", label: "我的角色", render: (item) => <Badge>{String(item.role)}</Badge> },
          { key: "updatedAt", label: "最近更新", render: (item) => new Date(String(item.updatedAt)).toLocaleDateString() }
        ]}
        items={projects as unknown as Record<string, unknown>[]}
      />
    </Page>
  );
}

import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { useAPI } from "@/lib/use-api";
import { APIStateView } from "@/components/api-state";
