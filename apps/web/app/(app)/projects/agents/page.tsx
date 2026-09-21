"use client";

import type { ComponentProps } from "react";
import { AgentConsole } from "@/components/agent-console";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";

type Data = ComponentProps<typeof AgentConsole>["sessions"];
export default function WorkspacePage() {
  return <StaticAPIPage<Data> endpoint={(query) => { const id = positiveParam(query); return id ? `/api/projects/${id}/agent-sessions` : null; }}>
    {(data, query) => <AgentConsole key={positiveParam(query)!} projectId={positiveParam(query)!} sessions={data} />}
  </StaticAPIPage>;
}
