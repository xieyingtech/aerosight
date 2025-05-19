export const requireAdminSession: typeof requireUserSession = async (
  event,
  opts
) => {
  const session = await requireUserSession(event);
  const isAdmin = await prisma.user.findUnique({
    where: { id: session.user.id },
    select: { admin: true },
  });
  if (!isAdmin) {
    throw createError({
      statusCode: opts?.statusCode ?? 403,
      message: opts?.message ?? "You must be an admin to access this resource",
    });
  }
  return session;
};
