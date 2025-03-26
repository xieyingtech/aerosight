import { z } from "zod";

const bodySchema = z.object({
  username: z.string(),
  password: z.string(),
});

export default defineEventHandler(async (event) => {
  const body = await readValidatedBody(event, bodySchema.parse);

  const user = await prisma.user.findUnique({
    where: {
      username: body.username,
    },
  });

  if (!user || !verifyPassword(user.password, body.password)) {
    throw createError({
      statusCode: 401,
      statusMessage: "Invalid username or password",
    });
  }

  await setUserSession(event, {
    user: { id: user.id },
  });
  return { id: user.id };
});
