export default defineEventHandler(async (event) => {
  // 验证用户是否已登录
  const { user } = await requireUserSession(event);
  
  // 获取团队namespace
  const namespace = event.context.params?.namespace;
  if (!namespace) {
    throw createError({
      statusCode: 400,
      statusMessage: 'Invalid team namespace'
    });
  }
  
  // 检查用户是否是该团队的成员
  const team = await prisma.team.findUnique({
    where: { namespace }
  });
  
  if (!team) {
    throw createError({
      statusCode: 404,
      statusMessage: 'Team not found'
    });
  }
  
  // 检查用户是否是该团队的成员
  const membership = await prisma.membership.findFirst({
    where: {
      userId: user.id,
      teamId: team.id
    }
  });
  
  // 如果不是成员且不是管理员，返回404
  if (!membership && !user.admin) {
    throw createError({
      statusCode: 404,
      statusMessage: 'Team not found'
    });
  }
  
  // 获取团队详细信息
  const teamDetails = await prisma.team.findUnique({
    where: { namespace },
    include: {
      projects: {
        include: {
          poi: true
        }
      },
      memberships: {
        include: {
          user: {
            select: {
              id: true,
              username: true
            }
          }
        }
      }
    }
  });
  
  return teamDetails;
});
