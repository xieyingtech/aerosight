import { listAdminUsers } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default async function AdminUsersPage() {
  const items = await listAdminUsers();
  return <Page title="用户管理"><DataTable columns={[{ key: "name", label: "姓名" }, { key: "email", label: "邮箱" }, { key: "phone", label: "手机号" }, { key: "role", label: "角色" }]} items={items} /></Page>;
}
