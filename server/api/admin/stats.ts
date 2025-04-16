export default defineEventHandler(async (event) => {
  await requireAdminSession(event);

  return {
    users: await prisma.user.count(),
    orgs: await prisma.organization.count(),
    projects: await prisma.project.count(),
  };
});
