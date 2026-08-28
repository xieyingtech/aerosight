import { z } from "zod";

const sensitiveKey = /authorization|cookie|credential|password|secret|token|api[-_]?key/i;

export const deviceAdapterInputSchema = z.object({
  name: z.string().trim().min(1).max(100),
  adapterType: z.enum(["simulator", "dji", "ros2", "mqtt", "mavlink", "rtsp", "gb28181"]),
  vendor: z.string().trim().max(100).optional(),
  protocolVersion: z.string().trim().min(1).max(50).default("1"),
  credentials: z.record(z.string(), z.string().max(16_384)).optional(),
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
  mqttUsername: z.string().trim().min(1).max(255),
  mqttPassword: z.string().min(1).max(16_384),
  appId: z.string().trim().min(1).max(255),
  appKey: z.string().min(1).max(16_384),
  appLicense: z.string().min(1).max(16_384),
  mediaPublishUser: z.string().trim().min(1).max(255),
  mediaPublishPassword: z.string().min(1).max(16_384),
  ntpServerHost: z.string().trim().min(1).max(253),
  ntpServerPort: z.coerce.number().int().min(1).max(65535).default(123),
  gatewaySerials: z.array(z.string().trim().min(1).max(100)).min(1).max(100)
});

export type DeviceAdapterInput = z.infer<typeof deviceAdapterInputSchema>;
export type DjiAdapterSetupInput = z.infer<typeof djiAdapterSetupInputSchema>;

export const djiCredentialUpdateSchema = z.object({
  mqttUsername: z.string().max(255).optional(), mqttPassword: z.string().max(16_384).optional(),
  appId: z.string().max(255).optional(), appKey: z.string().max(16_384).optional(),
  appLicense: z.string().max(16_384).optional(), mediaPublishUser: z.string().max(255).optional(),
  mediaPublishPassword: z.string().max(16_384).optional()
}).strict();

export type DjiCredentialUpdate = z.infer<typeof djiCredentialUpdateSchema>;

export function nonEmptyDjiCredentialUpdates(input: DjiCredentialUpdate) {
  return Object.fromEntries(Object.entries(input).filter(([, value]) => value?.trim())) as Record<string, string>;
}

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

export function buildDjiConfigurationSummary(input: Pick<DjiAdapterSetupInput,
  "gatewaySerials" | "mqttEndpoint" | "ntpServerHost" | "ntpServerPort">, clientId: string) {
  return {
    gateway_sn: input.gatewaySerials,
    mqtt_broker: {
      address: input.mqttEndpoint,
      client_id: clientId,
      username: "[ENCRYPTED]",
      password: "[ENCRYPTED]",
      enable_tls: input.mqttEndpoint.startsWith("mqtts://")
    },
    config: {
      app_id: "[ENCRYPTED]",
      app_key: "[ENCRYPTED]",
      app_license: "[ENCRYPTED]",
      ntp_server_host: input.ntpServerHost,
      ntp_server_port: input.ntpServerPort
    }
  };
}
