import { apiFetch } from "@/lib/api";
import { Page } from "@/components/ui";

export default async function AdminPage() {
  const overview = await apiFetch<{ users: number; teams: number; projects: number }>("/api/admin/overview");
  return (
    <Page title="管理总览">
      <div className="grid gap-4 sm:grid-cols-3">
        {[
          ["用户", overview.users],
          ["团队", overview.teams],
          ["项目", overview.projects]
        ].map(([label, value]) => (
          <div className="rounded-md border border-slate-200 bg-white p-4" key={label}>
            <p className="text-sm text-slate-500">{label}</p>
            <p className="mt-2 text-3xl font-semibold">{value}</p>
          </div>
        ))}
      </div>
    </Page>
  );
}
