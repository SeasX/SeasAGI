import { z } from "zod";

export const urlSchema = z
  .string()
  .min(1, "URL is required")
  .refine((val) => {
    try {
      new URL(val);
      return true;
    } catch {
      return false;
    }
  }, "Invalid URL format");

export const apiKeySchema = z
  .string()
  .min(1, "API Key is required")
  .max(512, "API Key is too long");

export const emailSchema = z
  .string()
  .email("Invalid email address");

export const passwordSchema = z
  .string()
  .min(6, "Password must be at least 6 characters")
  .max(128, "Password is too long");

export const displayNameSchema = z
  .string()
  .min(1, "Display name is required")
  .max(100, "Display name is too long");

export const channelNameSchema = z
  .string()
  .min(1, "Channel name is required")
  .max(100, "Channel name is too long");

export const comboNameSchema = z
  .string()
  .min(1, "Combo name is required")
  .max(100, "Combo name is too long")
  .regex(/^[a-zA-Z0-9_-]+$/, "Combo name can only contain letters, numbers, hyphens and underscores");

export const modelNameSchema = z
  .string()
  .min(1, "Model name is required")
  .max(200, "Model name is too long");

export const channelFormSchema = z.object({
  display_name: channelNameSchema,
  base_url: urlSchema,
  api_key: z.string().optional(),
  provider_type: z.string().min(1, "Provider type is required"),
});

export const comboFormSchema = z.object({
  name: comboNameSchema,
  strategy: z.enum(["fallback", "round_robin"]),
  sticky_uses: z.number().int().min(1).max(100),
  steps: z.array(z.object({
    model: modelNameSchema,
    channel_id: z.string().optional(),
  })).min(1, "At least one step is required"),
});

export const loginFormSchema = z.object({
  email: emailSchema,
  password: passwordSchema,
});

export const registerFormSchema = z.object({
  email: emailSchema,
  password: passwordSchema,
  displayName: displayNameSchema,
});

export type ValidationErrors = Record<string, string>;

export function validateForm<T>(schema: z.ZodSchema<T>, data: unknown): { success: true; data: T } | { success: false; errors: ValidationErrors } {
  const result = schema.safeParse(data);
  if (result.success) {
    return { success: true, data: result.data };
  }
  const errors: ValidationErrors = {};
  for (const issue of result.error.issues) {
    const key = issue.path.join(".");
    if (!errors[key]) {
      errors[key] = issue.message;
    }
  }
  return { success: false, errors };
}
