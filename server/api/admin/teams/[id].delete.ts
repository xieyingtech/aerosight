export default defineEventHandler(async (event) => {
  await requireAdminSession(event);

  const { id } = getRouterParams(event);
  if (isNaN(parseInt(id))) {
    throw createError({
      statusCode: 400,
      message: "Invalid team ID",
    });
  }

  return await prisma.team.delete({
    where: { id: parseInt(id) },
  });
});
