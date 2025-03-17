export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);
  return await prisma.membership.findMany({
    where: {
      userId: user.id,
    },
  });
});
