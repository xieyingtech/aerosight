"use client";

import { useAPI } from "@/lib/use-api";
import { APIStateView } from "@/components/api-state";
import { Page } from "@/components/page";
import { NewProjectForm } from "./new-project-form";

export default function NewProjectPage() {
  const state = useAPI<Array<{ id: number; name: string }>>("/api/teams?scope=managed");
  return (
    <Page description="选择一个你可管理的团队并填写项目名称" title="新建项目">
      <APIStateView state={state}>{(teams) => <NewProjectForm teams={teams} />}</APIStateView>
    </Page>
  );
}
