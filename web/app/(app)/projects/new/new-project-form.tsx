"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import type { ManagedTeam } from "@/lib/api";
import { browserApi } from "@/lib/browser-api";

export function NewProjectForm({ teams }: { teams: ManagedTeam[] }) {
  const router = useRouter();
  const [teamId, setTeamId] = useState(teams[0]?.id ?? 0);
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  return (
    <form
      className="max-w-xl space-y-4 rounded-md border border-slate-200 bg-white p-4"
      onSubmit={async (event) => {
        event.preventDefault();
        const response = await browserApi("/api/projects", {
          method: "POST",
          body: JSON.stringify({ teamId, name })
        });
        if (!response.ok) {
          const body = await response.json().catch(() => ({ message: "请求失败" }));
          setError(body.message);
          return;
        }
        router.push("/projects");
        router.refresh();
      }}
    >
      <label className="block text-sm">
        <span className="font-medium">所属团队</span>
        <select className="mt-1 h-10 w-full rounded-md border border-slate-300 px-3" value={teamId} onChange={(event) => setTeamId(Number(event.target.value))}>
          {teams.map((team) => (
            <option key={team.id} value={team.id}>
              {team.name}
            </option>
          ))}
        </select>
      </label>
      <label className="block text-sm">
        <span className="font-medium">项目名称</span>
        <input className="mt-1 h-10 w-full rounded-md border border-slate-300 px-3" value={name} onChange={(event) => setName(event.target.value)} />
      </label>
      {error ? <p className="text-sm text-red-600">{error}</p> : null}
      <button className="h-9 rounded-md bg-sky-700 px-3 text-sm font-medium text-white" disabled={!teams.length} type="submit">
        创建项目
      </button>
    </form>
  );
}
