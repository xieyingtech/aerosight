import { z } from "zod";

export const ALERT_AUTOMATION_MODES = ["manual", "agent-on-demand", "agent-auto-draft", "follow-up-draft"] as const;
export type AlertAutomationMode = (typeof ALERT_AUTOMATION_MODES)[number];

export const alertAutomationPolicyInputSchema = z.object({
  mode: z.enum(ALERT_AUTOMATION_MODES).default("manual")
}).strict();

export function isAutomaticAlertMode(mode: AlertAutomationMode) {
  return mode === "agent-auto-draft" || mode === "follow-up-draft";
}
