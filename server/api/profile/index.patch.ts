import { db, schema } from "@nuxthub/db";
import { eq } from "drizzle-orm";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);

  const body = await readBody(event);
  const payload = z
    .object({
      name: z
        .string()
        .trim()
        .min(1, "errors.validation.userName.required")
        .optional(),
      email: z.string().trim().optional(),
      phone: z.string().trim().optional(),
    })
    .parse(body);

  return db
    .update(schema.users)
    .set({
      ...(payload.name !== undefined ? { name: payload.name } : {}),
      ...(payload.email !== undefined ? { email: payload.email || null } : {}),
      ...(payload.phone !== undefined ? { phone: payload.phone || null } : {}),
      updatedAt: new Date(),
    })
    .where(eq(schema.users.id, user.id))
    .returning({
      id: schema.users.id,
      name: schema.users.name,
      email: schema.users.email,
      phone: schema.users.phone,
      role: schema.users.role,
      createdAt: schema.users.createdAt,
      updatedAt: schema.users.updatedAt,
    });
});