export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);
  const namespace = event.context.params?.namespace;
  const projectId = parseInt(event.context.params?.id || '');
  const poiId = parseInt(event.context.params?.poiId || '');

  if (isNaN(projectId) || isNaN(poiId)) {
    throw createError({
      statusCode: 400,
      message: '无效的ID参数',
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

  // 检查项目是否存在且属于该团队
  const project = await prisma.project.findFirst({
    where: {
      id: projectId,
      teamId: team.id,
    },
  });

  if (!project) {
    throw createError({
      statusCode: 404,
      message: '项目不存在',
    });
  }

  // 检查布防点是否存在且属于该项目
  const existingPoi = await prisma.pOI.findFirst({
    where: {
      id: poiId,
      projectId,
    },
  });

  if (!existingPoi) {
    throw createError({
      statusCode: 404,
      message: '布防点不存在',
    });
  }

  // 删除布防点
  await prisma.pOI.delete({
    where: {
      id: poiId,
    },
  });

  return { success: true };
});
