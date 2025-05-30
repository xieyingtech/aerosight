export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);
  const namespace = event.context.params?.namespace;
  const id = parseInt(event.context.params?.id || '');

  if (isNaN(id)) {
    throw createError({
      statusCode: 400,
      message: '无效的项目ID',
    });
  }

  // 查找团队
  const team = await prisma.team.findUnique({
    where: { namespace },
  });

  if (!team) {
    throw createError({
      statusCode: 404,
      message: '团队不存在',
    });
  }

  // 检查用户是否是团队成员
  const teamMember = await prisma.teamMember.findFirst({
    where: {
      userId: user.id,
      teamId: team.id,
    },
  });

  // Fetch the full user object to check systemAdmin status
  const fullUser = await prisma.user.findUnique({ where: { id: user.id } });

  if (!teamMember && !fullUser?.systemAdmin) {
    throw createError({
      statusCode: 403,
      message: '您不是该团队成员或系统管理员',
    });
  }

  // 先删除项目关联的布防点
  await prisma.pOI.deleteMany({
    where: {
      projectId: id,
    },
  });

  // 删除项目
  await prisma.project.delete({
    where: {
      id,
      teamId: team.id,
    },
  });

  return { success: true };
});
