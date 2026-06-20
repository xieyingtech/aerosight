import Link from "next/link";
import { Plus } from "lucide-react";
import { apiFetch, type TeamListItem } from "@/lib/api";
import { Badge, DataTable, Page } from "@/components/ui";
import { TeamCreateForm } from "./team-create-form";

export default async function TeamsPage({
  searchParams
}: {
  searchParams: Promise<{ search?: string }>;
}) {
  const params = await searchParams;
  const query = new URLSearchParams();
  if (params.search) query.set("search", params.search);
  const teams = await apiFetch<TeamListItem[]>(`/api/teams${query.size ? `?${query}` : ""}`);

  return (
    <Page
      actions={<TeamCreateForm />}
      description={`共 ${teams.length} 个团队`}
      title="团队"
    >
      <form action="/teams" className="flex gap-2">
        <input
          className="h-9 min-w-0 flex-1 rounded-md border border-slate-300 bg-white px-3 text-sm"
          defaultValue={params.search ?? ""}
          name="search"
          placeholder="按团队名称搜索"
        />
        <button className="h-9 rounded-md border border-slate-300 bg-white px-3 text-sm font-medium" type="submit">
          搜索
        </button>
      </form>
      <DataTable
        columns={[
          {
            key: "name",
            label: "团队",
            render: (item) => (
              <Link className="font-medium text-sky-700 hover:underline" href={`/teams/${String(item.id)}`}>
                {String(item.name)}
              </Link>
            )
          },
          { key: "role", label: "我的角色", render: (item) => <Badge>{roleLabel(String(item.role))}</Badge> },
          { key: "memberCount", label: "成员数" },
          { key: "createdAt", label: "创建时间", render: (item) => new Date(String(item.createdAt)).toLocaleDateString() }
        ]}
        items={teams as unknown as Record<string, unknown>[]}
      />
    </Page>
  );
}

function roleLabel(role: string) {
  if (role === "owner") return "所有者";
  if (role === "admin") return "管理员";
  return "成员";
}
