export function browserApi(path: string, init: RequestInit = {}) {
  const base = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
  const headers = new Headers(init.headers);
  if (init.body && !headers.has("content-type")) {
    headers.set("content-type", "application/json");
  }
  return fetch(`${base}${path}`, {
    ...init,
    headers,
    credentials: "include"
  });
}
