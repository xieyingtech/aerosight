import { apiFetch } from "@/lib/api";
import { DataTable, Page } from "@/components/ui";

export default async function AdminUsersPage() {
  const items = await apiFetch<Record<string, unknown>[]>("/api/admin/users");
  return <Page title="用户管理"><DataTable columns={[{ key: "name", label: "姓名" }, { key: "email", label: "邮箱" }, { key: "phone", label: "手机号" }, { key: "role", label: "角色" }]} items={items} /></Page>;
}
