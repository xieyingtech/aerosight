export type OperationDiagnostic = {
  id: string;
  deviceId?: number | null;
  kind: "command" | "connection" | "stream";
  severity: "info" | "warning" | "error";
  title: string;
  reason: string;
  status: string;
  occurredAt: string | Date | null;
};

export function diagnosticPresentation(item: OperationDiagnostic) {
  const falselySuccessful = ["acknowledged", "live", "online"].includes(item.status)
    && item.severity === "error";
  if (falselySuccessful) throw new Error("DIAGNOSTIC_FALSE_SUCCESS");
  return {
    ...item,
    label: item.kind === "command" ? "命令" : item.kind === "connection" ? "连接" : "直播",
    actionable: item.status === "unknown" || item.status === "timed_out" || item.status === "failed"
      || item.status === "nacked" || item.status === "degraded"
  };
}
