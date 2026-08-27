import { isIP } from "node:net";
import { lookup } from "node:dns/promises";

export type ResolvedAddress = { address: string; family: 4 | 6 };
export type HostResolver = (hostname: string) => Promise<ResolvedAddress[]>;

export type OutboundPolicy = {
  allowedHosts: string[];
  allowPrivateAddresses?: boolean;
  resolver?: HostResolver;
  maxRedirects?: number;
};

function hostAllowed(hostname: string, patterns: string[]) {
  const host = hostname.toLowerCase().replace(/\.$/, "");
  return patterns.some((pattern) => {
    const normalized = pattern.toLowerCase().replace(/\.$/, "");
    return normalized.startsWith("*.")
      ? host.endsWith(normalized.slice(1)) && host !== normalized.slice(2)
      : host === normalized;
  });
}

function parseIPv4(address: string) {
  const parts = address.split(".").map(Number);
  return parts.length === 4 && parts.every((part) => Number.isInteger(part) && part >= 0 && part <= 255) ? parts : null;
}

export function isRestrictedAddress(address: string) {
  const mapped = address.toLowerCase().match(/^::ffff:(\d+\.\d+\.\d+\.\d+)$/)?.[1];
  const ipv4 = parseIPv4(mapped ?? address);
  if (ipv4) {
    const [a, b] = ipv4;
    return a === 0 || a === 10 || a === 127 || a >= 224
      || (a === 100 && b >= 64 && b <= 127)
      || (a === 169 && b === 254)
      || (a === 172 && b >= 16 && b <= 31)
      || (a === 192 && (b === 0 || b === 168))
      || (a === 198 && (b === 18 || b === 19));
  }
  const normalized = address.toLowerCase().split("%")[0];
  return normalized === "::" || normalized === "::1"
    || normalized.startsWith("fc") || normalized.startsWith("fd")
    || /^fe[89ab]/.test(normalized) || normalized.startsWith("ff");
}

const systemResolver: HostResolver = async (hostname) => {
  if (isIP(hostname)) return [{ address: hostname, family: isIP(hostname) as 4 | 6 }];
  const addresses = await lookup(hostname, { all: true, verbatim: true });
  return addresses.map(({ address, family }) => ({ address, family: family as 4 | 6 }));
};

export async function assertSafeOutboundUrl(rawUrl: string, policy: OutboundPolicy) {
  let url: URL;
  try { url = new URL(rawUrl); } catch { throw new Error("OUTBOUND_URL_INVALID"); }
  if (url.protocol !== "https:") throw new Error("OUTBOUND_HTTPS_REQUIRED");
  if (url.username || url.password) throw new Error("OUTBOUND_URL_CREDENTIALS_FORBIDDEN");
  if (!hostAllowed(url.hostname, policy.allowedHosts)) throw new Error("OUTBOUND_HOST_NOT_ALLOWED");
  const addresses = await (policy.resolver ?? systemResolver)(url.hostname);
  if (!addresses.length) throw new Error("OUTBOUND_DNS_EMPTY");
  if (!policy.allowPrivateAddresses && addresses.some(({ address }) => isRestrictedAddress(address))) {
    throw new Error("OUTBOUND_ADDRESS_RESTRICTED");
  }
  return { url, addresses };
}

export type SafeTransportResponse = { status: number; location?: string };
export type SafeTransport = (target: { url: URL; addresses: ResolvedAddress[] }) => Promise<SafeTransportResponse>;

export async function followSafeRedirects(rawUrl: string, policy: OutboundPolicy, transport: SafeTransport) {
  let current = rawUrl;
  const visited: string[] = [];
  for (let redirect = 0; redirect <= (policy.maxRedirects ?? 5); redirect += 1) {
    const target = await assertSafeOutboundUrl(current, policy);
    visited.push(target.url.toString());
    const response = await transport(target);
    if (response.status < 300 || response.status >= 400 || !response.location) return { ...response, url: target.url, visited };
    current = new URL(response.location, target.url).toString();
  }
  throw new Error("OUTBOUND_REDIRECT_LIMIT_EXCEEDED");
}
