import { db, schema } from "@nuxthub/db";
import { and, eq, ne } from "drizzle-orm";
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
      name: z
        .string()
        .trim()
        .min(1, "errors.validation.teamName.required")
        .optional(),
      ownerUserId: z.coerce
        .number()
        .int()
        .positive("errors.validation.ownerUserId.required")
        .optional(),
    })
    .parse(body);

  return db.transaction(async (tx) => {
    if (payload.name !== undefined) {
      await tx
        .update(schema.teams)
        .set({
          name: payload.name,
          updatedAt: new Date(),
        })
        .where(eq(schema.teams.id, id));
    }

    if (payload.ownerUserId !== undefined) {
      const owner = await tx
        .select({ id: schema.users.id })
        .from(schema.users)
        .where(eq(schema.users.id, payload.ownerUserId))
        .limit(1);

      if (!owner.length) {
        throw createError({
          statusCode: 400,
          message: "errors.validation.ownerUserId.required",
        });
      }

      await tx
        .update(schema.teamMembers)
        .set({ role: "admin" })
        .where(
          and(
            eq(schema.teamMembers.teamId, id),
            eq(schema.teamMembers.role, "owner"),
            ne(schema.teamMembers.userId, payload.ownerUserId),
          ),
        );

      await tx
        .insert(schema.teamMembers)
        .values({
          teamId: id,
          userId: payload.ownerUserId,
          role: "owner",
        })
        .onConflictDoUpdate({
          target: [schema.teamMembers.teamId, schema.teamMembers.userId],
          set: { role: "owner" },
        });
    }

    return tx.select().from(schema.teams).where(eq(schema.teams.id, id));
  });
});
