import Link from "next/link";
import { Plus } from "lucide-react";
import { apiFetch, type ProjectListItem } from "@/lib/api";
import { Badge, Button, DataTable, Page } from "@/components/ui";

export default async function ProjectsPage({
  searchParams
}: {
  searchParams: Promise<{ scope?: string; search?: string }>;
}) {
  const params = await searchParams;
  const query = new URLSearchParams();
  if (params.scope) query.set("scope", params.scope);
  if (params.search) query.set("search", params.search);
  const projects = await apiFetch<ProjectListItem[]>(`/api/projects${query.size ? `?${query}` : ""}`);

  return (
    <Page
      actions={
        <Button href="/projects/new">
          <Plus size={16} />
          新建项目
        </Button>
      }
      description={`共 ${projects.length} 个项目`}
      title="项目"
    >
      <nav className="flex gap-2 text-sm">
        <Link className="rounded-md border border-slate-300 bg-white px-3 py-1.5" href="/projects">
          全部
        </Link>
        <Link className="rounded-md border border-slate-300 bg-white px-3 py-1.5" href="/projects?scope=joined">
          我参与的
        </Link>
        <Link className="rounded-md border border-slate-300 bg-white px-3 py-1.5" href="/projects?scope=managed">
          我管理的
        </Link>
      </nav>
      <DataTable
        columns={[
          {
            key: "name",
            label: "项目",
            render: (item) => (
              <div>
                <Link className="font-medium text-sky-700 hover:underline" href={`/projects/${String(item.id)}`}>
                  <span className="text-slate-500">{String(item.teamName)}/</span>
                  {String(item.name)}
                </Link>
                <p className="mt-1 text-xs text-slate-500">{String(item.description ?? "暂无描述")}</p>
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
