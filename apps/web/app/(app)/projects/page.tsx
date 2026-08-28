import Link from "next/link";
import { Plus } from "lucide-react";
import { listProjects } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export default async function ProjectsPage({
  searchParams
}: {
  searchParams: Promise<{ search?: string }>;
}) {
  const params = await searchParams;
  const projects = await listProjects("", params.search);

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
                <Link className="font-medium text-primary hover:underline" href={`/projects/${String(item.id)}`}>
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
