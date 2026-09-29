// Only watch runs created by a successful, explicitly authorized flight/workflow
// operation in this tab. Historical receipts must never trigger navigation.
export function agentFlightRunId(tool: string, status: string, input: Record<string, unknown> | undefined, receipt: unknown): number | null {
  if (status !== "succeeded" || !["launch_flight", "submit_flight", "run_task"].includes(tool)) return null;
  const result = receipt && typeof receipt === "object" ? receipt as Record<string, unknown> : {};
  const output = result.output && typeof result.output === "object" ? result.output as Record<string, unknown> : {};
  const id = tool === "submit_flight" ? input?.taskRunId : tool === "launch_flight" ? output.runId : output.taskRunId;
  return typeof id === "number" && Number.isSafeInteger(id) && id > 0 ? id : null;
}
