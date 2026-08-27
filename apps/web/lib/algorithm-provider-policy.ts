import { z } from "zod";

export const algorithmProviderInputSchema = z.object({
  name: z.string().trim().min(1).max(120),
  providerType: z.enum(["http-json", "kserve-v2", "ogc-processes", "ai-sdk"]),
  baseUrl: z.url(),
  secretRef: z.string().regex(/^secret:\/\/[a-zA-Z0-9._/-]+$/).nullable().optional(),
  authType: z.enum(["none", "bearer", "api-key-header", "basic", "signed"]),
  allowedHeaders: z.array(z.string().regex(/^[A-Za-z0-9-]+$/)).max(20).default([]),
  timeoutSeconds: z.number().int().min(1).max(3600),
  concurrencyLimit: z.number().int().min(1).max(1000),
  rateLimitPerMinute: z.number().int().min(1).max(1_000_000)
}).strict().superRefine((input, context) => {
  if (input.authType !== "none" && !input.secretRef) {
    context.addIssue({ code: "custom", path: ["secretRef"], message: "authenticated providers require a secret reference" });
  }
});

export type AlgorithmProviderInput = z.infer<typeof algorithmProviderInputSchema>;

export function publicAlgorithmProvider<T extends { secretRef?: string | null }>(provider: T) {
  const { secretRef, ...publicFields } = provider;
  return { ...publicFields, secretConfigured: Boolean(secretRef) };
}
