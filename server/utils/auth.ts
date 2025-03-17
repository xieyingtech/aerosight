export default async function requireAdminSession(
  event: Parameters<typeof requireUserSession>[0],
  opts?: Parameters<typeof requireUserSession>[1]
) {
  const session = await requireUserSession(event);
  if (!session.user.admin) {
    throw createError({
      statusCode: opts?.statusCode ?? 403,
      message: opts?.message ?? "You must be an admin to access this resource",
    });
  }
  return session;
}
