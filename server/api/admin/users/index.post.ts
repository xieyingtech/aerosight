import { requireAdminSession } from "~/server/utils/auth";

export default defineEventHandler(async (event) => {
  await requireAdminSession(event);

  const body = await readBody(event);

  // 验证必填字段
  if (!body.username || !body.password) {
    throw createError({
      statusCode: 400,
      message: "Username and password are required",
    });
  }

  return await prisma.user.create({
    data: {
      username: body.username,
      password: await hashPassword(body.password),
      phone: body.phone,
      email: body.email,
      admin: body.admin,
    },
  });
});
