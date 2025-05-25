export const requireAdminSession: typeof requireUserSession = async (
  event,
  opts
) => {
  const session = await requireUserSession(event);
  const user = await prisma.user.findUnique({
    where: { id: session.user.id },
    select: { admin: true },
  });
  if (!user?.admin) {
    throw createError({
      statusCode: opts?.statusCode ?? 403,
      message: opts?.message ?? "You must be an admin to access this resource",
    });
  }
  return session;
};
