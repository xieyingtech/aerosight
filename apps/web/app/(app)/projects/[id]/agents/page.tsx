import { listAgentSessions } from "@/lib/agent-sessions";
import { AgentConsole } from "@/components/agent-console";
import { Page } from "@/components/page";

export default async function AgentsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId=Number(id);const sessions=await listAgentSessions(projectId);
  return <Page title="时空智能体" description="基于当前项目证据查询态势、生成草案，并通过受保护控制面请求调度。"><AgentConsole projectId={projectId} sessions={sessions}/></Page>;
}
