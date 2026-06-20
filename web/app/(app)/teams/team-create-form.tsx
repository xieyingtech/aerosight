"use client";

import { Plus } from "lucide-react";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { browserApi } from "@/lib/browser-api";

export function TeamCreateForm() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  return (
    <>
      <button
        className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-sky-700 px-3 text-sm font-medium text-white shadow-sm hover:bg-sky-800"
        onClick={() => setOpen(true)}
        type="button"
      >
        <Plus size={16} />
        新建团队
      </button>
      {open ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 px-4">
          <form
            className="w-full max-w-md rounded-md border border-slate-200 bg-white p-5 shadow-lg"
            onSubmit={async (event) => {
              event.preventDefault();
              setPending(true);
              setError("");
              const response = await browserApi("/api/teams", {
                method: "POST",
                body: JSON.stringify({ name })
              });
              setPending(false);
              if (!response.ok) {
                const body = await response.json().catch(() => ({ message: "请求失败" }));
                setError(body.message);
                return;
              }
              setOpen(false);
              setName("");
              router.refresh();
            }}
          >
            <h2 className="text-lg font-semibold">新建团队</h2>
            <p className="mt-1 text-sm text-slate-500">创建团队后你将成为团队拥有者。</p>
            <label className="mt-4 block text-sm">
              <span className="font-medium">团队名称</span>
              <input
                className="mt-1 h-10 w-full rounded-md border border-slate-300 px-3"
                onChange={(event) => setName(event.target.value)}
                placeholder="请输入团队名称"
                value={name}
              />
            </label>
            {error ? <p className="mt-3 text-sm text-red-600">{error}</p> : null}
            <div className="mt-5 flex justify-end gap-2">
              <button className="h-9 rounded-md px-3 text-sm font-medium hover:bg-slate-100" onClick={() => setOpen(false)} type="button">
                取消
              </button>
              <button className="h-9 rounded-md bg-sky-700 px-3 text-sm font-medium text-white disabled:opacity-50" disabled={pending} type="submit">
                创建团队
              </button>
            </div>
          </form>
        </div>
      ) : null}
    </>
  );
}
