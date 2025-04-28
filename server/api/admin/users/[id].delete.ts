import { requireAdminSession } from "~/server/utils/auth";

export default defineEventHandler(async (event) => {
  await requireAdminSession(event);

  const { id } = getRouterParams(event);
  if (isNaN(parseInt(id))) {
    throw createError({
      statusCode: 400,
      message: "Invalid user ID",
    });
  }

  // 验证用户是否存在
  const user = await prisma.user.findUnique({
    where: { id: parseInt(id) },
  });

  if (!user) {
    throw createError({
      statusCode: 404,
      message: "User not found",
    });
  }

  // 删除用户关联的所有membership
  await prisma.membership.deleteMany({
    where: { userId: parseInt(id) },
  });

  return await prisma.user.delete({
    where: { id: parseInt(id) },
  });
});
