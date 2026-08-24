"use client";

import { useActionState } from "react";
import { createProjectAction } from "@/app/actions";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

type ManagedTeam = { id: number; name: string };

export function NewProjectForm({ teams }: { teams: ManagedTeam[] }) {
  const [state, action, pending] = useActionState(createProjectAction, {});

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
