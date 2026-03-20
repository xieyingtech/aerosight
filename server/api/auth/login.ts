import { db, schema } from "@nuxthub/db";
import { eq } from "drizzle-orm";

export default defineEventHandler(async (event) => {
  const body = await readBody(event);

  const user = await db.query.users.findFirst({
    where: eq(schema.users.email, body.email),
  });

  if (
    !user ||
    !user.password ||
    !(await verifyPassword(user.password, body.password))
  ) {
    throw createError({
      statusCode: 401,
      message: "Invalid email or password",
    });
  }

  return setUserSession(event, {
    user: { id: user.id, name: user.name, role: user.role },
  });
});
