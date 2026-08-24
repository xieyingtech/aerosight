export async function register() {
  if (process.env.NEXT_RUNTIME !== "nodejs") return;

  const { ensureDefaultAdmin } = await import("@/lib/bootstrap");
  await ensureDefaultAdmin();
}
