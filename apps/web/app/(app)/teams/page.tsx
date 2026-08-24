import Link from "next/link";
import { listTeams } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { TeamCreateForm } from "./team-create-form";

export default async function TeamsPage({
  searchParams
}: {
  searchParams: Promise<{ search?: string }>;
}) {
  const params = await searchParams;
  const teams = await listTeams(params.search);

  return (
    <Page
      actions={<TeamCreateForm />}
      description={`共 ${teams.length} 个团队`}
      title="团队"
    >
      <form action="/teams" className="flex gap-2">
        <Input
          className="min-w-0 flex-1"
          defaultValue={params.search ?? ""}
          name="search"
          placeholder="按团队名称搜索"
        />
        <Button type="submit" variant="outline">搜索</Button>
      </form>
      <DataTable
        columns={[
          {
            key: "name",
            label: "团队",
            render: (item) => (
              <Link className="font-medium text-primary hover:underline" href={`/teams/${String(item.id)}`}>
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
