import Link from "next/link";
import { Building2, Package, Shield, User } from "lucide-react";
import { getSession } from "@/lib/api";
import { LogoutButton } from "@/components/logout-button";

export async function AppShell({ children }: { children: React.ReactNode }) {
  const session = await getSession();
  return (
    <div className="min-h-screen">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4">
          <Link className="font-semibold tracking-tight" href="/">
            AeroSight
          </Link>
          <nav className="flex items-center gap-2 text-sm">
            <Link className="inline-flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-slate-100" href="/projects">
              <Package size={16} />
              项目
            </Link>
            <Link className="inline-flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-slate-100" href="/teams">
              <Building2 size={16} />
              团队
            </Link>
            {session?.user.role === "admin" ? (
              <Link className="inline-flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-slate-100" href="/admin">
                <Shield size={16} />
                管理
              </Link>
            ) : null}
            {session ? (
              <>
                <Link className="inline-flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-slate-100" href="/profile">
                  <User size={16} />
                  {session.user.name}
                </Link>
                <LogoutButton />
              </>
            ) : (
              <Link className="rounded-md border border-slate-300 px-3 py-1.5" href="/login">
                登录
              </Link>
            )}
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-6">{children}</main>
    </div>
  );
}
