import { isIP } from "node:net";
import { lookup } from "node:dns/promises";

import { isRestrictedAddress, type HostResolver, type ResolvedAddress } from "./outbound-url-policy.ts";

export const deviceNetworkProfileEndpointFields = [
  "mqttEndpoint",
  "apiPublicBaseUrl",
  "websocketPublicUrl",
  "mediaIngestBaseUrl",
  "mediaPlaybackBaseUrl"
] as const;

export type DeviceNetworkProfileEndpointField = typeof deviceNetworkProfileEndpointFields[number];

export type DeviceNetworkProfileInput = {
  mode: "lan" | "public";
  mqttEndpoint: string;
  apiPublicBaseUrl: string;
  websocketPublicUrl: string;
  mediaIngestBaseUrl: string;
  mediaPlaybackBaseUrl: string;
  tlsRequired: boolean;
  mqttAnonymous: boolean;
  credentialProvided: boolean;
};

export type DeviceNetworkProfileIssue = {
  field: DeviceNetworkProfileEndpointField | "tlsRequired" | "mqttAnonymous" | "credential";
  code: string;
};

export type ValidatedDeviceNetworkEndpoint = {
  field: DeviceNetworkProfileEndpointField;
  url: URL;
  addresses: ResolvedAddress[];
};

export type DeviceNetworkProfileValidation = {
  valid: boolean;
  issues: DeviceNetworkProfileIssue[];
  endpoints: Partial<Record<DeviceNetworkProfileEndpointField, ValidatedDeviceNetworkEndpoint>>;
};

const endpointSchemes: Record<DeviceNetworkProfileEndpointField, { lan: string[]; public: string[] }> = {
  mqttEndpoint: { lan: ["mqtt:", "mqtts:"], public: ["mqtts:"] },
  apiPublicBaseUrl: { lan: ["http:", "https:"], public: ["https:"] },
  websocketPublicUrl: { lan: ["ws:", "wss:"], public: ["wss:"] },
  mediaIngestBaseUrl: { lan: ["rtmp:", "rtmps:"], public: ["rtmps:"] },
  mediaPlaybackBaseUrl: { lan: ["http:", "https:"], public: ["https:"] }
};

function parseIPv4(address: string) {
  const parts = address.split(".").map(Number);
  return parts.length === 4 && parts.every((part) => Number.isInteger(part) && part >= 0 && part <= 255)
    ? parts
    : null;
}

export function isUnroutableDeviceAddress(address: string) {
  const mapped = address.toLowerCase().match(/^::ffff:(\d+\.\d+\.\d+\.\d+)$/)?.[1];
  const ipv4 = parseIPv4(mapped ?? address);
  if (ipv4) {
    const [first, second] = ipv4;
    return first === 0 || first === 127 || (first === 169 && second === 254) || first >= 224;
  }
  const normalized = address.toLowerCase().split("%")[0];
  return normalized === "::" || normalized === "::1" || /^fe[89ab]/.test(normalized) || normalized.startsWith("ff");
}

function isLocalhostName(hostname: string) {
  const normalized = hostname.toLowerCase().replace(/\.$/, "");
  return normalized === "localhost" || normalized.endsWith(".localhost");
}

const systemResolver: HostResolver = async (hostname) => {
  if (isIP(hostname)) return [{ address: hostname, family: isIP(hostname) as 4 | 6 }];
  const addresses = await lookup(hostname, { all: true, verbatim: true });
  return addresses.map(({ address, family }) => ({ address, family: family as 4 | 6 }));
};

function addIssue(issues: DeviceNetworkProfileIssue[], field: DeviceNetworkProfileIssue["field"], code: string) {
  if (!issues.some((issue) => issue.field === field && issue.code === code)) issues.push({ field, code });
}

export async function validateDeviceNetworkProfile(
  input: DeviceNetworkProfileInput,
  options: { resolver?: HostResolver } = {}
): Promise<DeviceNetworkProfileValidation> {
  const issues: DeviceNetworkProfileIssue[] = [];
  const endpoints: DeviceNetworkProfileValidation["endpoints"] = {};
  const resolver = options.resolver ?? systemResolver;
  const resolvedHosts = new Map<string, Promise<ResolvedAddress[]>>();
  const resolveHost = (hostname: string) => isIP(hostname)
    ? Promise.resolve([{ address: hostname, family: isIP(hostname) as 4 | 6 }])
    : Promise.resolve(resolver(hostname));

  if (input.mode === "public") {
    if (!input.tlsRequired) addIssue(issues, "tlsRequired", "PUBLIC_TLS_REQUIRED");
    if (input.mqttAnonymous) addIssue(issues, "mqttAnonymous", "PUBLIC_MQTT_ANONYMOUS_FORBIDDEN");
    if (!input.credentialProvided) addIssue(issues, "credential", "PUBLIC_CREDENTIAL_REQUIRED");
  }

  await Promise.all(deviceNetworkProfileEndpointFields.map(async (field) => {
    let url: URL;
    try {
      url = new URL(input[field]);
    } catch {
      addIssue(issues, field, "ENDPOINT_URL_INVALID");
      return;
    }

    if (!endpointSchemes[field][input.mode].includes(url.protocol)) {
      addIssue(issues, field, input.mode === "public" ? "PUBLIC_TLS_SCHEME_REQUIRED" : "ENDPOINT_SCHEME_UNSUPPORTED");
    }
    if (url.username || url.password) addIssue(issues, field, "ENDPOINT_INLINE_CREDENTIALS_FORBIDDEN");
    if (isLocalhostName(url.hostname)) {
      addIssue(issues, field, "ENDPOINT_LOOPBACK_FORBIDDEN");
      return;
    }

    try {
      const resolution = resolvedHosts.get(url.hostname) ?? resolveHost(url.hostname);
      resolvedHosts.set(url.hostname, resolution);
      const addresses = await resolution;
      if (!addresses.length) {
        addIssue(issues, field, "ENDPOINT_DNS_EMPTY");
        return;
      }
      if (addresses.some(({ address }) => isUnroutableDeviceAddress(address))) {
        addIssue(issues, field, "ENDPOINT_UNROUTABLE_ADDRESS");
      }
      if (input.mode === "public" && addresses.some(({ address }) => isRestrictedAddress(address))) {
        addIssue(issues, field, "PUBLIC_ADDRESS_REQUIRED");
      }
      endpoints[field] = { field, url, addresses };
    } catch {
      addIssue(issues, field, "ENDPOINT_DNS_FAILED");
    }
  }));

  return { valid: issues.length === 0, issues, endpoints };
}
