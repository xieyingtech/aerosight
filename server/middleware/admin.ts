export default defineEventHandler(async (event) => {
  const path = getRequestURL(event).pathname;

  if (!path.startsWith("/api/admin")) {
    return;
  }

  const { user } = await requireUserSession(event);

  if (user.role !== "admin") {
    throw createError({
      statusCode: 403,
      message: "errors.auth.forbidden",
    });
  }
});
