import { requireAdminSession } from "~/server/utils/auth";

export default defineEventHandler(async (event) => {
  await requireAdminSession(event);
  const body = await readBody(event);
  try {
    return await prisma.user.create({
      data: body,
    });
  } catch (e: any) {
    throw createError(e);
  }
});
