export default async function requireAdminSession(
  event: Parameters<typeof requireUserSession>[0],
  opts?: Parameters<typeof requireUserSession>[1]
) {
  const { user } = await requireUserSession(event);
  if (!user.admin) {
    throw createError({
      statusCode: opts?.statusCode ?? 403,
      message: opts?.message ?? "You must be an admin to access this resource",
    });
  }
  return { user };
}
