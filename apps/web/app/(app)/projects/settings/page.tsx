"use client";
import { SettingsIcon } from "lucide-react";

import { Page } from "@/components/page";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";

export default function ProjectSettingsPage() {
 return <StaticAPIPage<{role:string}> endpoint={(query)=>{const id=positiveParam(query);return id?`/api/projects/${id}`:null;}}>
 {(project)=>{if(project.role==="member") return <p className="p-4 text-sm text-destructive" role="alert">你没有项目管理权限。</p>;
  return (
    <Page description="功能开关、安全策略和项目级配置" title="项目设置">
      <div className="flex min-h-64 flex-col items-center justify-center rounded-lg border border-dashed bg-muted/20 p-8 text-center">
        <SettingsIcon className="mb-3 size-8 text-muted-foreground" />
        <p className="font-medium">项目配置中心</p>
        <p className="mt-1 text-sm text-muted-foreground">所有外部能力默认关闭，管理员配置并验证后才会启用。</p>
      </div>
    </Page>
  );
 }}</StaticAPIPage>;
}
