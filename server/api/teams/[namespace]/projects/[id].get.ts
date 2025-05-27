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

  // 先查找团队
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
  const membership = await prisma.membership.findFirst({
    where: {
      userId: user.id,
      teamId: team.id,
    },
  });

  if (!membership && !user.admin) {
    throw createError({
      statusCode: 403,
      message: '您不是该团队成员',
    });
  }

  // 获取项目详情
  const project = await prisma.project.findFirst({
    where: {
      id,
      teamId: team.id,
    },
    include: {
      poi: true, // 包含布防点信息
    },
  });

  if (!project) {
    throw createError({
      statusCode: 404,
      message: '项目不存在',
    });
  }

  return project;
});
