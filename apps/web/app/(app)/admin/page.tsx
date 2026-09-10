"use client";

import { useAPI } from "@/lib/use-api";
import { APIStateView } from "@/components/api-state";
import { Page } from "@/components/page";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default function AdminPage() {
  const state = useAPI<{ users: number; teams: number; projects: number }>("/api/admin/overview");
  return <APIStateView state={state}>{(overview) => (
    <Page title="管理总览">
      <div className="grid gap-4 sm:grid-cols-3">
        {[
          ["用户", overview.users],
          ["团队", overview.teams],
          ["项目", overview.projects]
        ].map(([label, value]) => (
          <Card key={label}>
            <CardHeader><CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle></CardHeader>
            <CardContent><p className="text-3xl font-semibold">{value}</p></CardContent>
          </Card>
        ))}
      </div>
    </Page>
  )}</APIStateView>;
}
