"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { APIError, getSession, type SessionUser } from "@/lib/api-client";
import { Button } from "@/components/ui/button";

const SessionContext = createContext<SessionUser | null>(null);
export const useSessionUser = () => useContext(SessionContext);

export function SessionProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [user, setUser] = useState<SessionUser | null>(null);
  const [failed, setFailed] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    const expired = () => { controller.abort(); setUser(null); router.replace("/login"); };
    window.addEventListener("aerosight:unauthenticated", expired);
    setFailed(false);
    getSession(controller.signal).then(({ user }) => { if (!controller.signal.aborted) setUser(user); }).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      if (error instanceof APIError && error.status === 401) expired(); else setFailed(true);
    });
    return () => { controller.abort(); window.removeEventListener("aerosight:unauthenticated", expired); };
  }, [router, revision]);
  if (failed) return <div className="p-6" role="alert"><p>暂时无法获取登录状态。</p><Button onClick={() => setRevision((value) => value + 1)} variant="outline">重试</Button></div>;
  if (!user) return <p className="p-6 text-sm text-muted-foreground" role="status">正在检查登录状态…</p>;
  return <SessionContext.Provider value={user}>{children}</SessionContext.Provider>;
}

export function AdminGuard({ children }: { children: ReactNode }) {
  const user = useSessionUser();
  return user?.role === "admin" ? children : <p className="p-4 text-sm text-destructive" role="alert">你没有平台管理权限。</p>;
}
