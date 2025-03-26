const requireAdminSession: typeof requireUserSession = async (event, opts) => {
  const session = await requireUserSession(event);
  if (!session.user.admin) {
    throw createError({
      statusCode: opts?.statusCode ?? 403,
      message: opts?.message ?? "You must be an admin to access this resource",
    });
  }
  return session;
};

export default requireAdminSession;
