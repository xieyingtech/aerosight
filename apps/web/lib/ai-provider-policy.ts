import { z } from "zod";

export const aiProviderInputSchema = z.object({
  name: z.string().trim().min(1).max(120),
  providerType: z.literal("openai"),
  baseUrl: z.union([z.url(), z.literal("")]).optional(),
  modelId: z.string().trim().min(1).max(255),
  apiKey: z.string().max(16_384).optional(),
  enabled: z.boolean().default(false),
  isDefault: z.boolean().default(false)
}).strict().superRefine((input, context) => {
  if (input.isDefault && !input.enabled) {
    context.addIssue({ code: "custom", path: ["isDefault"], message: "default AI provider must be enabled" });
  }
});

export type AIProviderInput = z.infer<typeof aiProviderInputSchema>;

export function normalizedAIAPIKey(input: Pick<AIProviderInput, "apiKey">) {
  return input.apiKey?.trim() || null;
}
