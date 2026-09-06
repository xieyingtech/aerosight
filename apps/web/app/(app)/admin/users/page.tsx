"use client";

import { useAPI } from "@/lib/use-api";
import { APIStateView } from "@/components/api-state";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default function AdminUsersPage() {
  const state = useAPI<Record<string, unknown>[]>("/api/admin/users");
  return <APIStateView state={state}>{(items) => <Page title="用户管理"><DataTable columns={[{ key: "name", label: "姓名" }, { key: "email", label: "邮箱" }, { key: "phone", label: "手机号" }, { key: "role", label: "角色" }]} items={items} /></Page>}</APIStateView>;
}
