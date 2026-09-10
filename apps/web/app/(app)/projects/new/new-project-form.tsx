"use client";

import { useActionState } from "react";
import { apiJSON, APIError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

type ManagedTeam = { id: number; name: string };

export function NewProjectForm({ teams }: { teams: ManagedTeam[] }) {
  const [state, action, pending] = useActionState(async (_: { error?: string }, form: FormData): Promise<{ error?: string }> => {
    const name = String(form.get("name") ?? "").trim();
    const teamId = Number(form.get("teamId"));
    if (!name || name.length > 100) return { error: "请输入 1–100 字符的项目名称" };
    if (!Number.isSafeInteger(teamId) || teamId <= 0) return { error: "请选择团队" };
    try { const project = await apiJSON<{ id: number }>("/api/projects", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ teamId, name }) }); window.location.assign(`/projects/detail/?projectId=${project.id}`); return {}; }
    catch (error) { return { error: error instanceof APIError && error.status === 403 ? "你没有在此团队创建项目的权限。" : "创建项目失败，请重试。" }; }
  }, {});

  return (
    <Card className="max-w-xl">
      <CardContent>
    <form action={action} className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="teamId">所属团队</Label>
        <Select defaultValue={teams[0] ? String(teams[0].id) : undefined} name="teamId">
          <SelectTrigger className="w-full" id="teamId"><SelectValue placeholder="请选择团队" /></SelectTrigger>
          <SelectContent>
          {teams.map((team) => (
            <SelectItem key={team.id} value={String(team.id)}>{team.name}</SelectItem>
          ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-2">
        <Label htmlFor="name">项目名称</Label>
        <Input id="name" name="name" required />
      </div>
      {state.error ? <p className="text-sm text-destructive">{state.error}</p> : null}
      <Button disabled={!teams.length || pending} size="lg" type="submit">
        创建项目
      </Button>
    </form>
      </CardContent>
    </Card>
  );
}
