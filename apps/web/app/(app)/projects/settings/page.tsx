"use client";

import { Page } from "@/components/page";
import { ProjectFeatureSettings } from "@/components/project-feature-settings";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
import type { FeatureSettings } from "@/lib/project-features";

export default function ProjectSettingsPage() {
  return <StaticAPIPage<FeatureSettings> endpoint={(query) => {
    const id = positiveParam(query);
    return id ? `/api/projects/${id}/feature-settings` : null;
  }}>
    {(data) => <Page description="按功能分组管理整个项目的开关" title="项目设置">
      <ProjectFeatureSettings key={data.projectId} data={data} />
    </Page>}
  </StaticAPIPage>;
}
