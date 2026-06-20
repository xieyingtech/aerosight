import { cookies } from "next/headers";

export type SessionUser = {
  id: number;
  name: string;
  role: "user" | "admin";
};

export type ProjectListItem = {
  id: number;
  name: string;
  description: string | null;
  teamName: string;
  role: "owner" | "admin" | "member";
  updatedAt: string;
};

export type Project = ProjectListItem & {
  teamId: number;
};

export type ManagedTeam = {
  id: number;
  name: string;
};

export type TeamRole = "owner" | "admin" | "member";

export type TeamListItem = {
  id: number;
  name: string;
  role: TeamRole;
  memberCount: number;
  createdAt: string;
  updatedAt: string;
};

export type TeamDetail = {
  team: TeamListItem;
  projects: {
    id: number;
    name: string;
    description: string | null;
    updatedAt: string;
  }[];
};

const apiBase = process.env.API_BASE_URL ?? process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const cookieStore = await cookies();
  const cookie = cookieStore.toString();
  const headers = new Headers(init.headers);
  if (cookie) {
    headers.set("cookie", cookie);
  }
  if (init.body && !headers.has("content-type")) {
    headers.set("content-type", "application/json");
  }

  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    headers,
    cache: "no-store"
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({ message: "errors.generic" }));
    throw new ApiError(response.status, body.message ?? "errors.generic");
  }
  return response.json() as Promise<T>;
}

export async function getSession() {
  try {
    return await apiFetch<{ user: SessionUser }>("/api/auth/session");
  } catch {
    return null;
  }
}
