import { z } from "zod";

export const ALERT_AUTOMATION_MODES = ["manual", "agent-on-demand", "agent-auto-draft", "follow-up-draft"] as const;
export type AlertAutomationMode = (typeof ALERT_AUTOMATION_MODES)[number];

export const alertAutomationPolicyInputSchema = z.object({
  mode: z.enum(ALERT_AUTOMATION_MODES).default("manual"),
  eventRuleVersionId: z.number().int().positive().nullable().default(null),
  config: z.record(z.string(),z.unknown()).default({})
}).strict();

export type AlertAutomationPolicyVersion = {
  version: number;
  status: "draft" | "published" | "retired";
  mode: AlertAutomationMode;
  eventRuleVersionId: number | null;
  config: Record<string,unknown>;
};

export function createAlertAutomationPolicyVersion(history: readonly AlertAutomationPolicyVersion[], rawInput: unknown): AlertAutomationPolicyVersion {
  if(history.some((version)=>version.status==="draft"))throw new Error("ALERT_AUTOMATION_DRAFT_EXISTS");
  const input=alertAutomationPolicyInputSchema.parse(rawInput);
  return Object.freeze({version:Math.max(0,...history.map((item)=>item.version))+1,status:"draft",...input});
}

export function publishAlertAutomationPolicyVersion(history: readonly AlertAutomationPolicyVersion[], versionNumber:number){
  const target=history.find((version)=>version.version===versionNumber);
  if(!target||target.status!=="draft")throw new Error("ALERT_AUTOMATION_DRAFT_NOT_PUBLISHABLE");
  return history.map((version)=>version.version===versionNumber?{...version,status:"published" as const}:version.status==="published"?{...version,status:"retired" as const}:version);
}
