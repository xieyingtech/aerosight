import Link from "next/link";
import { getTeam } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";

export default async function TeamDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const detail = await getTeam(Number(id));
  return (
    <Page
      description={`${roleLabel(detail.team.role)} · ${detail.team.memberCount} 名成员`}
      title={detail.team.name}
    >
      <DataTable
        columns={[
          {
            key: "name",
            label: "项目",
            render: (item) => (
              <div>
                <Link className="font-medium text-sky-700 hover:underline" href={`/projects/${String(item.id)}`}>
                  {String(item.name)}
                </Link>
                <p className="mt-1 text-xs text-muted-foreground">{String(item.description ?? "暂无描述")}</p>
              </div>
            )
          },
          { key: "updatedAt", label: "最近更新", render: (item) => new Date(String(item.updatedAt)).toLocaleDateString() }
        ]}
        items={detail.projects as unknown as Record<string, unknown>[]}
      />
      <div className="text-sm text-muted-foreground">
        我的角色 <Badge>{roleLabel(detail.team.role)}</Badge>
      </div>
    </Page>
  );
}

function roleLabel(role: string) {
  if (role === "owner") return "所有者";
  if (role === "admin") return "管理员";
  return "成员";
}
