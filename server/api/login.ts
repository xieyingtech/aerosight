import bcrypt from "bcryptjs";
import { z } from "zod";

const bodySchema = z.object({
  username: z.string(),
  password: z.string(),
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
      statusMessage: "Invalid username or password",
    });
  }

  await setUserSession(event, {
    user: { username, admin: user.admin },
  });
  return { username };
});

declare module "#auth-utils" {
  interface User {
    username: string;
    admin: boolean;
  }
}
