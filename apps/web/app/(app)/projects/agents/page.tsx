"use client";

import type { ComponentProps } from "react";
import { AgentConsole } from "@/components/agent-console";
import { Page } from "@/components/page";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";

type Data = ComponentProps<typeof AgentConsole>["sessions"];
export default function WorkspacePage() {
  return <StaticAPIPage<Data> endpoint={(query) => { const id = positiveParam(query); return id ? `/api/projects/${id}/agent-sessions` : null; }}>
    {(data, query, reload) => <Page title="时空智能体" description="基于当前项目证据查询态势、生成草案，并通过受保护控制面请求调度。"><AgentConsole key={positiveParam(query)!} projectId={positiveParam(query)!} sessions={data} onChanged={reload} /></Page>}
  </StaticAPIPage>;
}
