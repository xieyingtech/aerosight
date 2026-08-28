import { z } from "zod";

export const algorithmProviderInputSchema = z.object({
  name: z.string().trim().min(1).max(120),
  providerType: z.enum(["http-json", "kserve-v2", "ogc-processes", "ai-sdk"]),
  baseUrl: z.url(),
  authType: z.enum(["none", "bearer", "api-key-header", "basic", "signed"]),
  credential: z.string().max(16_384).optional(),
  username: z.string().trim().max(255).optional(),
  allowedHeaders: z.array(z.string().regex(/^[A-Za-z0-9-]+$/)).max(20).default([]),
  timeoutSeconds: z.number().int().min(1).max(3600),
  concurrencyLimit: z.number().int().min(1).max(1000),
  rateLimitPerMinute: z.number().int().min(1).max(1_000_000)
}).strict().superRefine((input, context) => {
  if (input.authType === "basic" && input.credential?.trim() && !input.username) {
    context.addIssue({ code: "custom", path: ["username"], message: "basic authentication requires a username" });
  }
});

export type AlgorithmProviderInput = z.infer<typeof algorithmProviderInputSchema>;

export function algorithmCredentialPayload(input: Pick<AlgorithmProviderInput, "authType" | "credential" | "username">) {
  const credential = input.credential?.trim();
  if (!credential) return null;
  if (input.authType === "bearer") return { token: credential };
  if (input.authType === "api-key-header") return { apiKey: credential };
  if (input.authType === "basic") return { username: input.username ?? "", password: credential };
  if (input.authType === "signed") return { secret: credential };
  return null;
}
