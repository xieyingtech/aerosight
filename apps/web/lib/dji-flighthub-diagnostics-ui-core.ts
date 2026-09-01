export type FlightHubDiagnosticStatus = "supported" | "empty" | "forbidden" | "not_applicable" | "unverified" | "degraded" | "failed";

export type FlightHubDiagnosticsPayload = {
  connector: { id: string; name: string; status: string; lastErrorCode: string | null; lastCheckedAt: string | Date | null };
  resourceWatermarks: Array<{
    resourceKind: string; status: string; attemptCount: number; lastErrorCode: string | null;
    lastStartedAt: string | Date | null; lastSucceededAt: string | Date | null; nextAttemptAt: string | Date | null;
  }>;
  capabilities: Array<{
    capabilityCode: string; status: FlightHubDiagnosticStatus; evidenceLevel: string; region: string; deployment: string;
    deviceModel: string | null; firmwareVersion: string | null; reason: string | null; endpointId: string | null;
    layers: Record<string, string>; verifiedAt: string | Date; expiresAt: string | Date | null;
  }>;
};

export const capabilityStatusLabels: Record<FlightHubDiagnosticStatus, string> = {
  supported: "支持",
  empty: "正常空状态",
  forbidden: "权限不足",
  not_applicable: "当前部署不适用",
  unverified: "待验证",
  degraded: "暂时降级",
  failed: "失败",
};

export const evidenceLevelLabels: Record<string, string> = {
  documented: "官方契约",
  fixture: "脱敏 Fixture",
  "live-read": "真实只读",
  "field-write": "现场写入",
};

export function connectorDiagnosticHealth(payload: FlightHubDiagnosticsPayload) {
  const statuses = payload.capabilities.map(({ status }) => status);
  if (statuses.includes("failed")) return { status: "failed", label: "存在失败能力" } as const;
  if (statuses.some((status) => ["degraded", "forbidden", "unverified"].includes(status))) {
    return { status: "degraded", label: "部分能力需处理" } as const;
  }
  return { status: "supported", label: "只读能力正常" } as const;
}
