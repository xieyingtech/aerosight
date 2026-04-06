import { db, schema } from "@nuxthub/db";
import { eq } from "drizzle-orm";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);

  const body = await readBody(event);
  const payload = z
    .object({
      name: z.string().trim().min(1, "errors.validation.teamName.required"),
      ownerUserId: z.coerce
        .number()
        .int()
        .positive("errors.validation.ownerUserId.required")
        .optional(),
    })
    .parse(body);

  const ownerUserId = payload.ownerUserId ?? user.id;

  const owner = await db
    .select({ id: schema.users.id })
    .from(schema.users)
    .where(eq(schema.users.id, ownerUserId))
    .limit(1);

  if (!owner.length) {
    throw createError({
      statusCode: 400,
      message: "errors.validation.ownerUserId.required",
    });
  }

  return db.transaction(async (tx) => {
    const created = await tx
      .insert(schema.teams)
      .values({
        name: payload.name,
      })
      .returning();

    if (created[0]) {
      await tx.insert(schema.teamMembers).values({
        teamId: created[0].id,
        userId: ownerUserId,
        role: "owner",
      });
    }

    return created;
  });
});
