import type { FlightHubProject } from "./dji-flighthub-client-core.ts";

export function defaultFlightHubProjectSelection(projects: FlightHubProject[]) {
  return projects.length === 1 ? projects[0]!.uuid : "";
}

export function flightHubErrorMessage(code: string | undefined) {
  switch (code) {
    case "credential_invalid": return "Token 无效或已撤销，请重新获取后再试。";
    case "scope_forbidden": return "当前 Token 无权访问该司空项目。";
    case "scope_not_found":
    case "project_access_changed": return "所选司空项目已不可访问，请重新验证 Token。";
    case "duplicate_connection": return "这个司空项目已连接到当前 AeroSight 项目。";
    case "rate_limited": return "司空接口请求过于频繁，请稍后重试。";
    case "request_timeout": return "司空接口响应超时，请检查公网出口后重试。";
    case "connector_disabled": return "连接器已断开，不能继续同步。";
    case "connector_not_disabled": return "连接器当前并非已断开状态，无需重新连接。";
    case "schema_incompatible": return "司空返回格式发生变化，已停止同步以保护现有设备数据。";
    case "configuration_unavailable": return "司空连接器尚未在当前部署中启用。";
    case "invalid_request": return "请求内容不完整，请检查输入。";
    default: return "司空服务暂时不可用，请稍后重试。";
  }
}

export function flightHubStatusLabel(status: string) {
  switch (status) {
    case "connecting": return "等待首次同步";
    case "connected": return "连接正常";
    case "degraded": return "同步降级";
    case "failed": return "需要处理";
    case "disabled": return "已断开";
    default: return status;
  }
}

export function discoveryStatusLabel(status: string) {
  switch (status) {
    case "discovered": return "待确认";
    case "managed": return "已纳管";
    case "conflicted": return "身份冲突";
    case "missing": return "本次未发现";
    case "ignored": return "已忽略";
    default: return status;
  }
}

export const flightHubReadOnlyCapabilities = ["目录读取", "状态读取"] as const;
export const flightHubUnavailableActions = ["任务下发", "返航", "机场调试", "直播控制"] as const;
