import { db, schema } from "@nuxthub/db";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  await requireUserSession(event);

  const body = await readBody(event);
  const payload = z
    .object({
      name: z.string().trim().min(1, "errors.validation.userName.required"),
      email: z.string().trim().optional(),
      phone: z.string().trim().optional(),
      role: z.enum(["user", "admin"]).default("user"),
      password: z.string().trim().min(1, "errors.validation.password.required"),
    })
    .parse(body);

  return db
    .insert(schema.users)
    .values({
      name: payload.name,
      email: payload.email || null,
      phone: payload.phone || null,
      role: payload.role,
      password: await hashPassword(payload.password),
    })
    .returning();
});
