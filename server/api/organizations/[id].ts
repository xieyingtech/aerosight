export default defineEventHandler(async (event) => {
  // 验证用户是否已登录
  const { user } = await requireUserSession(event);
  
  // 获取组织ID
  const id = parseInt(event.context.params?.id || '');
  if (isNaN(id)) {
    throw createError({
      statusCode: 400,
      statusMessage: 'Invalid organization ID'
    });
  }
  
  // 检查用户是否是该组织的成员
  const membership = await prisma.membership.findFirst({
    where: {
      userId: user.id,
      organizationId: id
    }
  });
  
  // 如果不是成员且不是管理员，返回404
  if (!membership && !user.admin) {
    throw createError({
      statusCode: 404,
      statusMessage: 'Organization not found'
    });
  }
  
  // 获取组织详细信息
  const organization = await prisma.organization.findUnique({
    where: { id },
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
  
  if (!organization) {
    throw createError({
      statusCode: 404,
      statusMessage: 'Organization not found'
    });
  }
  
  return organization;
});
