import { User } from "@prisma/client";
export default defineEventHandler(async (event) => {
  await requireAdminSession(event);

  const { id } = getRouterParams(event);
  if (isNaN(parseInt(id))) {
    throw createError({
      statusCode: 400,
      message: "Invalid user ID",
    });
  }

  const body = await readBody(event);

  if (!body.username) {
    throw createError({
      statusCode: 400,
      message: "Username is required",
    });
  }

  // 验证用户是否存在
  const userExists = await prisma.user.findUnique({
    where: { id: parseInt(id) },
  });

  if (!userExists) {
    throw createError({
      statusCode: 404,
      message: "User not found",
    });
  }

  const newUser: Partial<User> = {
    username: body.username,
    phone: body.phone,
    email: body.email,
    systemAdmin: body.admin, // Changed from admin to systemAdmin
  };

  if (body.password) {
    newUser.password = await hashPassword(body.password);
  }

  return await prisma.user.update({
    where: { id: parseInt(id) },
    data: newUser,
  });
});
