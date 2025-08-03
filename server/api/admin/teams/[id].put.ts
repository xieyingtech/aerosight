export default defineEventHandler(async (event) => {
  await requireAdminSession(event);

  const { id } = getRouterParams(event);
  if (isNaN(parseInt(id))) {
    throw createError({
      statusCode: 400,
      message: "Invalid team ID",
    });
  }

  const body = await readBody(event);

  if (!body.name) {
    throw createError({
      statusCode: 400,
      message: "Team name is required",
    });
  }

  return await prisma.team.update({
    where: { id: parseInt(id) },
    data: {
      name: body.name,
      description: body.description || null,
    },
  });
});
