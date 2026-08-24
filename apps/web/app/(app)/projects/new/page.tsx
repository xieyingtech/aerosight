import { listManagedTeams } from "@/lib/data";
import { Page } from "@/components/page";
import { NewProjectForm } from "./new-project-form";

export default async function NewProjectPage() {
  const teams = await listManagedTeams();
  return (
    <Page description="选择一个你可管理的团队并填写项目名称" title="新建项目">
      <NewProjectForm teams={teams} />
    </Page>
  );
}
