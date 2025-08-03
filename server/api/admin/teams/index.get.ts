export default defineEventHandler(async (event) => {
  await requireAdminSession(event);
  return await prisma.team.findMany();
});
