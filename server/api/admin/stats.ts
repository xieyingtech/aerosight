export default defineEventHandler(async (event) => {
  await requireAdminSession(event);

  return {
    users: await prisma.user.count(),
    teams: await prisma.team.count(),
    projects: await prisma.project.count(),
  };
});
