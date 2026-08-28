import "server-only";

import { ToolLoopAgent, stepCountIs, tool } from "ai";
import { z } from "zod";
import { createAgentExecutionContext } from "@/lib/agent-execution-context-core";
import { loadAgentProviderRegistry } from "@/lib/agent-provider-loader";
import { executeAgentReadTool } from "@/lib/agent-read-tools";
import type { AgentReadToolName } from "@/lib/agent-read-tools-core";
import { appendAgentMessage } from "@/lib/agent-sessions";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";

const emptyInput = z.object({}).strict();
const deviceInput = z.object({ deviceIds: z.array(z.number().int().positive()).max(100).optional() }).strict();
const limitInput = z.object({ limit: z.number().int().min(1).max(100).default(20) }).strict();

export async function runAgentChatTurn(input: { projectId: number; sessionId: number; content: string; requestId?: string | null }) {
  const { user, access } = await requireCurrentProjectPermission(input.projectId, "agent:use");
  const session = (await query<{ id: number }>(`select id from agent_sessions
    where id=$1 and project_id=$2 and started_by_user_id=$3 and status='open'`, [input.sessionId, input.projectId, user.id])).rows[0];
  if (!session) throw new Error("AGENT_SESSION_NOT_FOUND");
  await appendAgentMessage({ ...input, role: "user" });
  const history = (await query<{ role: "user" | "assistant"; content: string }>(`select role,content from (
    select message.id,message.role,message.content from agent_messages message
    where message.session_id=$1 and message.role in('user','assistant') order by message.id desc limit 20
  ) recent order by id`, [input.sessionId])).rows;
  const configured = await loadAgentProviderRegistry();
  const context = createAgentExecutionContext({ userId: user.id, teamId: access.teamId, projectId: input.projectId, sessionId: input.sessionId });
  const call = (name: AgentReadToolName, rawInput: unknown) => executeAgentReadTool(context, name, rawInput);
  const tools = {
    query_devices: tool({ description: "查询当前项目设备、类型、驱动、状态和数据新鲜度", inputSchema: deviceInput, execute: (value) => call("query_devices", value) }),
    query_tasks: tool({ description: "查询当前项目 Tasks 及其最近运行状态", inputSchema: limitInput, execute: (value) => call("query_missions", value) }),
    query_issues: tool({ description: "查询当前项目案件、状态、优先级和证据质量", inputSchema: limitInput, execute: (value) => call("query_events", value) }),
    query_assets: tool({ description: "查询当前项目可用数据资产及版本", inputSchema: emptyInput, execute: (value) => call("query_assets", value) }),
    query_tracks: tool({ description: "查询当前项目设备轨迹摘要", inputSchema: emptyInput, execute: (value) => call("query_tracks", { window: {}, ...value }) }),
    query_map_context: tool({ description: "查询当前项目地图态势摘要", inputSchema: emptyInput, execute: (value) => call("query_map_context", value) })
  };
  const agent = new ToolLoopAgent({
    id: "aerosight-project-copilot",
    model: configured.registry.languageModel(configured.modelId),
    instructions: "你是 AeroSight 项目 Copilot。先使用平台查询工具核对事实，再用中文回答。明确数据时间和质量；不得把旧告警事件说成案件，不得声称已执行设备或算法操作。需要操作时，只建议用户创建或启动 Task。",
    tools,
    stopWhen: stepCountIs(8)
  });
  const result = await agent.generate({ messages: history });
  const toolCalls = result.steps.flatMap((step) => step.toolResults.map((item) => {
    const output = item.output as { items?: Array<Record<string, unknown>>; truncated?: boolean };
    const evidenceRefs = (output?.items ?? []).slice(0, 20).flatMap((row) => {
      const reference = row.reference as { type?: string; id?: string; href?: string } | undefined;
      return reference?.type && reference.id ? [{ type: reference.type, id: reference.id, version: String(row.version ?? row.observedAt ?? "current"), href: reference.href }] : [];
    });
    return { name: item.toolName, status: "succeeded", summary: output?.truncated ? "结果已安全截断" : `返回 ${output?.items?.length ?? 0} 条项目内记录`, evidenceRefs };
  }));
  const assistant = await appendAgentMessage({ projectId: input.projectId, sessionId: input.sessionId, role: "assistant",
    content: result.text || "未生成可用回复。", toolCalls, requestId: input.requestId });
  return { ...assistant, content: result.text, modelId: configured.modelId };
}
