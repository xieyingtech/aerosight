export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);

  return await prisma.user.findUnique({
    where: { id: user.id },
    omit: {
      password: true,
    },
  });
});
