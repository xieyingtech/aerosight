import { actionsForCapability, type DeviceCapabilityAction } from "./device-capability-actions.ts";
import { actionPatternMatches, authorizeCapabilityAction } from "./device-command-core.ts";

export type DeviceCapabilityGrant = {
  scopeType: "project" | "device_type" | "device";
  deviceTypeId: string | null;
  deviceId: number | null;
  actionPattern: string;
  effect: "allow" | "deny";
};

export type ProjectedDeviceCapability = {
  code: string;
  availability: string;
  reason: string | null;
  risk: DeviceCapabilityAction["risk"];
  authorized: boolean;
  actions: Array<DeviceCapabilityAction & { enabled: boolean; unavailableReason: string | null }>;
};

export function projectDeviceCapabilities(input: {
  deviceId: number;
  deviceTypeId: string;
  deviceStatus: string;
  role: "owner" | "admin" | "member";
  capabilities: Array<Omit<ProjectedDeviceCapability, "authorized" | "actions">>;
  grants: DeviceCapabilityGrant[];
}): ProjectedDeviceCapability[] {
  return input.capabilities.map((capability) => {
    const matching = input.grants.filter((grant) => actionPatternMatches(grant.actionPattern, capability.code)
      && (grant.scopeType === "project"
        || (grant.scopeType === "device_type" && grant.deviceTypeId === input.deviceTypeId)
        || (grant.scopeType === "device" && grant.deviceId === input.deviceId)));
    let authorized = false;
    try {
      authorized = authorizeCapabilityAction({ role: input.role, action: capability.code, grants: matching });
    } catch {}
    const unavailableReason = !authorized ? "当前账号没有该操作权限"
      : capability.availability !== "available" ? capability.reason ?? "设备能力当前不可用"
        : input.deviceStatus !== "online" ? "设备不在线，暂时无法执行"
          : null;
    return {
      ...capability,
      authorized,
      actions: authorized ? actionsForCapability(capability.code, capability.risk)
        .map((action) => ({ ...action, enabled: unavailableReason === null, unavailableReason })) : []
    };
  });
}
