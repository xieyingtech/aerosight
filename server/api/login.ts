import bcrypt from "bcryptjs";
import { z } from "zod";
import prisma from "~/lib/prisma";

const bodySchema = z.object({
  username: z.string().min(3),
  password: z.string().min(8),
});

export default defineEventHandler(async (event) => {
  const { username, password } = await readValidatedBody(
    event,
    bodySchema.parse
  );

  const user = await prisma.user.findUnique({
    where: {
      username: username,
    },
  });

  if (!user || !bcrypt.compareSync(password, user.password)) {
    throw createError({
      statusCode: 401,
      message: "Invalid username or password",
    });
  }

  await setUserSession(event, { user: { username } });
  return { username };
});
