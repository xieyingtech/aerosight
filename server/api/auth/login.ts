import { db, schema } from "@nuxthub/db";
import { eq, or } from "drizzle-orm";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  const body = await readBody(event);
  const { email, phone, password } = z
    .object({
      email: z.string().optional(),
      phone: z.string().optional(),
      password: z
        .string("errors.validation.password.required")
        .min(1, "errors.validation.password.required"),
    })
    .refine((data) => Boolean(data.email || data.phone), {
      message: "errors.validation.username.required",
    })
    .parse(body);

  const user = await db.query.users.findFirst({
    where:
      email && phone
        ? or(eq(schema.users.email, email), eq(schema.users.phone, phone))
        : email
          ? eq(schema.users.email, email)
          : eq(schema.users.phone, phone!),
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
