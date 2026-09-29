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
    reason: ({
      FLIGHTHUB_LIVE_START_REJECTED_STREAMING_GATEWAY_NOT_FOUND: "司空未找到推流网关，请确认设备在线、飞行器已开机且相机可用（213003）。",
      FLIGHTHUB_LIVE_START_RESPONSE_UNKNOWN: "未能确认司空是否接受直播请求，请检查设备状态后重试。",
      FLIGHTHUB_LIVE_START_ACCEPTED: "司空已接受直播请求，正在连接视频。",
      FLIGHTHUB_LIVE_STOP_REMOTE_ACTIVE: "已停止本次观看；设备仍在推流，司空将在无观众后自动停止。",
    } as Record<string, string>)[item.reason] ?? item.reason,
    label: item.kind === "command" ? "命令" : item.kind === "connection" ? "连接" : "直播",
    actionable: item.status === "unknown" || item.status === "timed_out" || item.status === "failed"
      || item.status === "nacked" || item.status === "degraded"
  };
}
