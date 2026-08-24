export async function register() {
  if (process.env.NEXT_RUNTIME !== "nodejs") return;

  const { getWebRuntimeConfig } = await import("@/lib/runtime-config");
  getWebRuntimeConfig();
  const { ensureDefaultAdmin } = await import("@/lib/bootstrap");
  await ensureDefaultAdmin();
}
