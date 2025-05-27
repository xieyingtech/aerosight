export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);

  // 查找用户的所有membership和对应的团队
  const memberships = await prisma.membership.findMany({
    where: {
      userId: user.id,
    },
    include: {
      team: true,
    },
  });

  // 构建团队数组，每个团队包含对应的membership
  const teams = memberships.map(membership => ({
    ...membership.team,
    membership: {
      id: membership.id,
      roles: membership.roles
    }
  }));

  return teams;
});
