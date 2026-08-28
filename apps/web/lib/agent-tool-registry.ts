import { z } from "zod";
import type { ProjectPermission } from "./project-permission-policy.ts";
import { inspectionTaskDraftInputSchema, issueDraftInputSchema, reportDraftInputSchema } from "./agent-draft-tools-core.ts";
import { agentMissionStartInputSchema } from "./agent-mission-start-core.ts";

export type AgentToolRisk = "read-only" | "draft" | "protected";
export type AgentToolConfirmation = "never" | "required";

export type AgentToolDefinition = Readonly<{
  description: string;
  inputSchema: z.ZodType;
  risk: AgentToolRisk;
  permission: ProjectPermission;
  confirmation: AgentToolConfirmation;
}>;

const timeWindowSchema = z.object({
  from: z.string().datetime().optional(),
  to: z.string().datetime().optional()
}).strict();

export const agentToolRegistry = Object.freeze({
  query_devices: {
    description: "查询当前项目设备及其状态",
    inputSchema: z.object({ deviceIds: z.array(z.number().int().positive()).max(100).optional() }).strict(),
    risk: "read-only",
    permission: "project:view",
    confirmation: "never"
  },
  query_missions: {
    description: "查询当前项目巡检任务及运行",
    inputSchema: z.object({ window: timeWindowSchema.optional(), limit: z.number().int().min(1).max(100).default(20) }).strict(),
    risk: "read-only",
    permission: "project:view",
    confirmation: "never"
  },
  query_events: {
    description: "查询当前项目案件与证据",
    inputSchema: z.object({ eventIds: z.array(z.string().min(1)).max(100).optional(), window: timeWindowSchema.optional() }).strict(),
    risk: "read-only",
    permission: "project:view",
    confirmation: "never"
  },
  query_assets: {
    description: "查询当前项目媒体与数据资产元数据",
    inputSchema: z.object({ window: timeWindowSchema.optional(), kinds: z.array(z.string().min(1)).max(20).optional() }).strict(),
    risk: "read-only",
    permission: "project:view",
    confirmation: "never"
  },
  query_tracks: {
    description: "查询当前项目设备轨迹",
    inputSchema: z.object({ deviceIds: z.array(z.number().int().positive()).max(20).optional(), window: timeWindowSchema }).strict(),
    risk: "read-only",
    permission: "project:view",
    confirmation: "never"
  },
  query_map_context: {
    description: "查询当前项目地图区域与时空态势摘要",
    inputSchema: z.object({ window: timeWindowSchema.optional() }).strict(),
    risk: "read-only",
    permission: "project:view",
    confirmation: "never"
  },
  draft_inspection_task: {
    description: "创建当前项目的巡检任务草案",
    inputSchema: inspectionTaskDraftInputSchema,
    risk: "draft",
    permission: "mission:operate",
    confirmation: "never"
  },
  draft_report: {
    description: "创建当前项目的报告草案",
    inputSchema: reportDraftInputSchema,
    risk: "draft",
    permission: "agent:use",
    confirmation: "never"
  },
  draft_issue: {
    description: "创建当前项目的问题草案",
    inputSchema: issueDraftInputSchema,
    risk: "draft",
    permission: "issue:handle",
    confirmation: "never"
  },
  request_mission_start: {
    description: "通过平台安全控制面请求启动已发布任务",
    inputSchema: agentMissionStartInputSchema,
    risk: "protected",
    permission: "mission:operate",
    confirmation: "required"
  }
} satisfies Record<string, AgentToolDefinition>);

export type AgentToolName = keyof typeof agentToolRegistry;

export function agentToolRegistrySnapshot() {
  return Object.entries(agentToolRegistry).map(([name, definition]) => ({
    name,
    risk: definition.risk,
    permission: definition.permission,
    confirmation: definition.confirmation
  }));
}

export function parseAgentToolInput(name: AgentToolName, input: unknown): unknown {
  return agentToolRegistry[name].inputSchema.parse(input);
}
