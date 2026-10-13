"use client";

import { Page } from "@/components/page";
import { ProjectFeatureSettings } from "@/components/project-feature-settings";
import { ProjectNameSettings } from "@/components/project-name-settings";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
import type { FeatureSettings } from "@/lib/project-features";

export default function ProjectSettingsPage() {
  return <StaticAPIPage<FeatureSettings> endpoint={(query) => {
    const id = positiveParam(query);
    return id ? `/api/projects/${id}/feature-settings` : null;
  }}>
    {(data) => <Page description="管理项目基本信息和功能开关" title="项目设置">
      <ProjectNameSettings key={data.projectId} projectId={data.projectId} />
      <ProjectFeatureSettings key={data.projectId} data={data} />
    </Page>}
  </StaticAPIPage>;
}
