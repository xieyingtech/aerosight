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
  
  // 检查团队是否存在
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
  const teamMember = await prisma.teamMember.findFirst({
    where: {
      userId: user.id,
      teamId: team.id
    }
  });
  
  // Fetch the full user object to check systemAdmin status
  const fullUser = await prisma.user.findUnique({ where: { id: user.id } });

  // 如果不是成员且不是系统管理员，返回404 (or 403)
  if (!teamMember && !fullUser?.systemAdmin) {
    throw createError({
      statusCode: 403, // Changed to 403 for permission denied
      statusMessage: 'Access denied to this team'
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
      teamMembers: { // Changed from memberships
        include: {
          user: {
            select: {
              id: true,
              username: true,
              name: true, // include name if available/needed
            }
          },
          memberRoles: { // Include memberRoles and the actual role
            include: {
              role: {
                select: {
                  name: true,
                  // permissions: true, // Optionally include permissions
                }
              }
            }
          }
        }
      }
    }
  });

  if (!teamDetails) {
    // This case should ideally be caught by the team check above
    throw createError({
      statusCode: 404,
      statusMessage: 'Team not found'
    });
  }

  // Transform teamMembers to include a simple roles array for easier frontend consumption
  const transformedTeamDetails = {
    ...teamDetails,
    teamMembers: teamDetails.teamMembers.map(tm => ({
      id: tm.id, // TeamMember ID
      userId: tm.userId,
      teamId: tm.teamId,
      user: tm.user,
      roles: tm.memberRoles.map(mr => mr.role.name), // Array of role names
      // memberRoles: tm.memberRoles, // Optionally pass full memberRoles structure
    })),
  };
  
  return transformedTeamDetails;
});
