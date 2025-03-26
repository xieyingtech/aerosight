import { requireAdminSession } from "~/server/utils/auth";

export default defineEventHandler(async (event) => {
  await requireAdminSession(event);
  const { id } = getRouterParams(event);
  try {
    return await prisma.user.delete({
      where: { id: Number(id) },
    });
  } catch (e: any) {
    throw createError(e);
  }
});
