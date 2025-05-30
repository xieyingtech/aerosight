export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);

  // 查找用户的所有team memberships
  const teamMembers = await prisma.teamMember.findMany({
    where: {
      userId: user.id,
    },
    include: {
      team: true,
      memberRoles: {
        include: {
          role: true,
        },
      },
    },
  });

  // 构建团队数组，每个团队包含对应的teamMember信息和角色
  const teams = teamMembers.map(tm => ({
    ...tm.team,
    teamMember: { // Renamed from 'membership' for clarity with new schema
      id: tm.id,
      roles: tm.memberRoles.map(mr => mr.role.name), // Extract role names
    }
  }));

  return teams;
});
