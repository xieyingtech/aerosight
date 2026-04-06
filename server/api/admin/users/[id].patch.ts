import { db, schema } from "@nuxthub/db";
import { eq } from "drizzle-orm";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  const { user: _user } = await requireUserSession(event);

  const id = z.coerce
    .number()
    .int()
    .positive("errors.validation.id.required")
    .parse(getRouterParam(event, "id"));

  const body = await readBody(event);
  const payload = z
    .object({
      name: z.string().trim().min(1, "errors.validation.userName.required").optional(),
      email: z.string().trim().optional(),
      phone: z.string().trim().optional(),
      role: z.enum(["user", "admin"]).optional(),
      password: z.string().trim().min(1, "errors.validation.password.required").optional(),
    })
    .parse(body);

  return db
    .update(schema.users)
    .set({
      ...(payload.name !== undefined ? { name: payload.name } : {}),
      ...(payload.email !== undefined ? { email: payload.email || null } : {}),
      ...(payload.phone !== undefined ? { phone: payload.phone || null } : {}),
      ...(payload.role !== undefined ? { role: payload.role } : {}),
      ...(payload.password !== undefined
        ? { password: await hashPassword(payload.password) }
        : {}),
      updatedAt: new Date(),
    })
    .where(eq(schema.users.id, id))
    .returning();
});
