import { requireAdminSession } from "~/server/utils/auth";

export default defineEventHandler(async (event) => {
  await requireAdminSession(event);
  return await prisma.user.findMany({
    omit: {
      password: true,
    },
  });
});
