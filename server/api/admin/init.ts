export default defineEventHandler(async (event) => {
  const adminExists = await prisma.user.findFirst({
    where: {
      admin: true,
    },
  });

  if (adminExists) {
    throw createError({
      statusCode: 400,
      statusMessage: "Admin user already exists",
    });
  }

  const { email, username, password } = await readBody(event);
  return await prisma.user.create({
    data: {
      email,
      username,
      password: await hashPassword(password),
      admin: true,
    },
  });
});
