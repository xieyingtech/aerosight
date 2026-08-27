import { z } from "zod";

const runtimeConfigSchema = z.object({
  DATABASE_URL: z.preprocess(
    (value) => value ?? "",
    z.string().trim().min(1, "DATABASE_URL is required")
  ),
  AUTH_SECRET: z.preprocess(
    (value) => value ?? "",
    z.string().min(16, "AUTH_SECRET must contain at least 16 characters")
  ),
  OBJECT_STORAGE_LOCAL_ROOT: z.string().trim().min(1).optional(),
  ALGORITHM_ALLOWED_HOSTS: z.string().default(""),
  LOG_LEVEL: z.enum(["debug", "info", "warn", "error"]).default("info"),
  AI_PROVIDER: z.enum(["disabled", "openai"]).default("disabled"),
  AI_MODEL: z.string().trim().optional(),
  OPENAI_API_KEY: z.string().trim().optional()
}).superRefine((config,context)=>{
  if(config.AI_PROVIDER==="openai"&&!config.AI_MODEL)context.addIssue({code:"custom",path:["AI_MODEL"],message:"AI_MODEL is required when AI_PROVIDER=openai"});
  if(config.AI_PROVIDER==="openai"&&!config.OPENAI_API_KEY)context.addIssue({code:"custom",path:["OPENAI_API_KEY"],message:"OPENAI_API_KEY is required when AI_PROVIDER=openai"});
});

export type WebRuntimeConfig = {
  databaseUrl: string;
  authSecret: string;
  objectStorageLocalRoot: string | null;
  algorithmAllowedHosts: string[];
  logLevel: "debug" | "info" | "warn" | "error";
  aiProvider: "disabled" | "openai";
  aiModel: string | null;
  openaiApiKey: string | null;
};

export function parseWebRuntimeConfig(environment: Record<string, string | undefined>): WebRuntimeConfig {
  const parsed = runtimeConfigSchema.safeParse(environment);
  if (!parsed.success) {
    const details = parsed.error.issues.map((issue) => issue.message).join("; ");
    throw new Error(`Invalid AeroSight web configuration: ${details}`);
  }
  return {
    databaseUrl: parsed.data.DATABASE_URL,
    authSecret: parsed.data.AUTH_SECRET,
    objectStorageLocalRoot: parsed.data.OBJECT_STORAGE_LOCAL_ROOT ?? null,
    algorithmAllowedHosts: parsed.data.ALGORITHM_ALLOWED_HOSTS.split(",").map((host) => host.trim()).filter(Boolean),
    logLevel: parsed.data.LOG_LEVEL,
    aiProvider: parsed.data.AI_PROVIDER,
    aiModel: parsed.data.AI_MODEL ?? null,
    openaiApiKey: parsed.data.OPENAI_API_KEY ?? null
  };
}

let cachedConfig: WebRuntimeConfig | undefined;

export function getWebRuntimeConfig() {
  cachedConfig ??= parseWebRuntimeConfig(process.env);
  return cachedConfig;
}
