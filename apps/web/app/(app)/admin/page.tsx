import { getAdminOverview } from "@/lib/data";
import { Page } from "@/components/page";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default async function AdminPage() {
  const overview = await getAdminOverview();
  return (
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
  );
}
