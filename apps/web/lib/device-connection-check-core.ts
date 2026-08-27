import {
  deviceNetworkProfileEndpointFields,
  validateDeviceNetworkProfile,
  type DeviceNetworkProfileEndpointField,
  type DeviceNetworkProfileInput,
  type DeviceNetworkProfileIssue,
  type ValidatedDeviceNetworkEndpoint
} from "./device-network-profile.ts";
import type { HostResolver } from "./outbound-url-policy.ts";

export type DeviceEndpointProbe = (endpoint: ValidatedDeviceNetworkEndpoint) => Promise<void>;

export type DeviceConnectionDiagnostic = {
  field: DeviceNetworkProfileEndpointField;
  endpoint: string;
  status: "server_verified" | "failed" | "not_checked";
  code: string;
  deviceVerification: "pending" | "not_applicable";
};

export type DeviceConnectionCheck = {
  ok: boolean;
  status: "valid" | "invalid";
  serverVerification: "verified" | "failed";
  deviceVerification: "pending";
  checkedAt: string;
  diagnostics: DeviceConnectionDiagnostic[];
  policyIssues: DeviceNetworkProfileIssue[];
};

const deviceFacingFields = new Set<DeviceNetworkProfileEndpointField>([
  "mqttEndpoint",
  "apiPublicBaseUrl",
  "websocketPublicUrl",
  "mediaIngestBaseUrl"
]);

function safeEndpoint(endpoint: string) {
  try {
    const url = new URL(endpoint);
    return `${url.protocol}//${url.hostname}${url.port ? `:${url.port}` : ""}`;
  } catch {
    return "[invalid endpoint]";
  }
}

function policyCode(field: DeviceNetworkProfileEndpointField, issues: DeviceNetworkProfileIssue[]) {
  return issues.find((issue) => issue.field === field)?.code ?? "ENDPOINT_POLICY_REJECTED";
}

export async function checkDeviceNetworkConnection(
  profile: DeviceNetworkProfileInput,
  dependencies: { probe: DeviceEndpointProbe; resolver?: HostResolver; now?: () => Date }
): Promise<DeviceConnectionCheck> {
  const validation = await validateDeviceNetworkProfile(profile, { resolver: dependencies.resolver });
  const diagnostics = await Promise.all(deviceNetworkProfileEndpointFields.map(async (field): Promise<DeviceConnectionDiagnostic> => {
    const summary = safeEndpoint(profile[field]);
    const endpoint = validation.endpoints[field];
    const deviceVerification = deviceFacingFields.has(field) ? "pending" as const : "not_applicable" as const;
    if (!endpoint || validation.issues.some((issue) => issue.field === field)) {
      return { field, endpoint: summary, status: "not_checked", code: policyCode(field, validation.issues), deviceVerification };
    }
    try {
      await dependencies.probe(endpoint);
      return { field, endpoint: summary, status: "server_verified", code: "ENDPOINT_REACHABLE", deviceVerification };
    } catch {
      return { field, endpoint: summary, status: "failed", code: "ENDPOINT_PROBE_FAILED", deviceVerification };
    }
  }));
  const ok = validation.valid && diagnostics.every((diagnostic) => diagnostic.status === "server_verified");
  return {
    ok,
    status: ok ? "valid" : "invalid",
    serverVerification: ok ? "verified" : "failed",
    deviceVerification: "pending",
    checkedAt: (dependencies.now ?? (() => new Date()))().toISOString(),
    diagnostics,
    policyIssues: validation.issues
  };
}
