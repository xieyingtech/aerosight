import { db, schema } from "@nuxthub/db";
import { eq } from "drizzle-orm";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  const body = await readBody(event);
  const { username, password } = z
    .object({
      username: z
        .string("errors.validation.username.required")
        .min(1, "errors.validation.username.required"),
      password: z
        .string("errors.validation.password.required")
        .min(1, "errors.validation.password.required"),
    })
    .parse(body);

  const user = await db.query.users.findFirst({
    where: eq(schema.users.name, username),
  });

  if (
    !user ||
    !user.password ||
    !(await verifyPassword(user.password, password))
  ) {
    throw createError({
      statusCode: 401,
      message: "errors.auth.invalidCredentials",
    });
  }

  return setUserSession(event, {
    user: { id: user.id, name: user.name, role: user.role },
  });
});
