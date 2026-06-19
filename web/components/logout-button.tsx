"use client";

import { LogOut } from "lucide-react";
import { useRouter } from "next/navigation";
import { browserApi } from "@/lib/browser-api";

export function LogoutButton() {
  const router = useRouter();
  return (
    <button
      className="inline-flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-slate-100"
      onClick={async () => {
        await browserApi("/api/auth/logout", { method: "POST" });
        router.push("/login");
        router.refresh();
      }}
      type="button"
    >
      <LogOut size={16} />
      登出
    </button>
  );
}
