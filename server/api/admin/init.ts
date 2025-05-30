export default defineEventHandler(async (event) => {
  const adminExists = await prisma.user.findFirst({
    where: {
      systemAdmin: true, // Changed from admin
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
      systemAdmin: true, // Changed from admin
    },
  });
});
