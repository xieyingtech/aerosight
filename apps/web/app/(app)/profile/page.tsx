import { getProfileData } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default async function ProfilePage() {
  const { profile, teams } = await getProfileData();
  return (
    <Page description="查看账号基础信息和所属团队" title="个人中心">
      <div className="grid gap-5 lg:grid-cols-2">
        <DataTable columns={[{ key: "name", label: "姓名" }, { key: "email", label: "邮箱" }, { key: "phone", label: "手机号" }, { key: "role", label: "角色" }]} items={profile as unknown as Record<string, unknown>[]} />
        <DataTable columns={[{ key: "name", label: "团队" }, { key: "role", label: "角色" }, { key: "joinedAt", label: "加入时间" }]} items={teams} />
      </div>
    </Page>
  );
}
