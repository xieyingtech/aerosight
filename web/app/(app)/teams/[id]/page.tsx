import Link from "next/link";
import { apiFetch, type TeamDetail } from "@/lib/api";
import { Badge, DataTable, Page } from "@/components/ui";

export default async function TeamDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const detail = await apiFetch<TeamDetail>(`/api/teams/${id}`);
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
                <p className="mt-1 text-xs text-slate-500">{String(item.description ?? "暂无描述")}</p>
              </div>
            )
          },
          { key: "updatedAt", label: "最近更新", render: (item) => new Date(String(item.updatedAt)).toLocaleDateString() }
        ]}
        items={detail.projects as unknown as Record<string, unknown>[]}
      />
      <div className="text-sm text-slate-500">
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
