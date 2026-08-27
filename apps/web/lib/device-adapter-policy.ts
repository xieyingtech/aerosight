import { z } from "zod";

const sensitiveKey = /authorization|cookie|credential|password|secret|token|api[-_]?key/i;

export const deviceAdapterInputSchema = z.object({
  name: z.string().trim().min(1).max(100),
  adapterType: z.enum(["simulator", "dji", "ros2", "mqtt", "mavlink", "rtsp", "gb28181"]),
  vendor: z.string().trim().max(100).optional(),
  protocolVersion: z.string().trim().min(1).max(50).default("1"),
  secretRef: z.string().trim().regex(/^[a-z][a-z0-9+.-]*:\/\/.+/i).optional(),
  config: z.record(z.string(), z.unknown()).default({})
});

export const djiAdapterSetupInputSchema = z.object({
  name: z.string().trim().min(1).max(100),
  mode: z.enum(["lan", "public"]),
  mqttEndpoint: z.string().trim().min(1).max(500),
  apiPublicBaseUrl: z.string().trim().min(1).max(500),
  websocketPublicUrl: z.string().trim().min(1).max(500),
  mediaIngestBaseUrl: z.string().trim().min(1).max(500),
  mediaPlaybackBaseUrl: z.string().trim().min(1).max(500),
  tlsRequired: z.boolean(),
  mqttAnonymous: z.boolean().default(false),
  secretRef: z.string().trim().regex(/^[a-z][a-z0-9+.-]*:\/\/.+/i),
  ntpServerHost: z.string().trim().min(1).max(253),
  ntpServerPort: z.coerce.number().int().min(1).max(65535).default(123),
  gatewaySerials: z.array(z.string().trim().min(1).max(100)).min(1).max(100)
});

export type DeviceAdapterInput = z.infer<typeof deviceAdapterInputSchema>;
export type DjiAdapterSetupInput = z.infer<typeof djiAdapterSetupInputSchema>;

export function canManageDeviceAdapters(role: "owner" | "admin" | "member" | null) {
  return role === "owner" || role === "admin";
}

export function assertSupportedDeviceAdapterType(adapterType: DeviceAdapterInput["adapterType"]) {
  if (adapterType !== "simulator" && adapterType !== "dji") {
    throw new Error(`ADAPTER_TYPE_NOT_SUPPORTED:${adapterType}`);
  }
}

export function assertNoInlineSecrets(value: unknown, path = "config") {
  if (!value || typeof value !== "object") return;
  if (Array.isArray(value)) {
    value.forEach((item, index) => assertNoInlineSecrets(item, `${path}[${index}]`));
    return;
  }
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (sensitiveKey.test(key)) {
      throw new Error(`INLINE_SECRET_NOT_ALLOWED:${path}.${key}`);
    }
    assertNoInlineSecrets(item, `${path}.${key}`);
  }
}

export function publicDeviceAdapter<T extends { secretRef: string | null }>(adapter: T) {
  const { secretRef, ...safe } = adapter;
  return { ...safe, hasSecret: secretRef !== null };
}

export function buildDjiConfigurationSummary(input: Pick<DjiAdapterSetupInput,
  "gatewaySerials" | "mqttEndpoint" | "ntpServerHost" | "ntpServerPort">, clientId: string) {
  return {
    gateway_sn: input.gatewaySerials,
    mqtt_broker: {
      address: input.mqttEndpoint,
      client_id: clientId,
      username: "[SECRET_REF]",
      password: "[SECRET_REF]",
      enable_tls: input.mqttEndpoint.startsWith("mqtts://")
    },
    config: {
      app_id: "[SECRET_REF]",
      app_key: "[SECRET_REF]",
      app_license: "[SECRET_REF]",
      ntp_server_host: input.ntpServerHost,
      ntp_server_port: input.ntpServerPort
    }
  };
}
