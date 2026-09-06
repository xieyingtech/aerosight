"use client";

import { useActionState, useState } from "react";
import { Plus } from "lucide-react";
import { apiJSON } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function TeamCreateForm({ onCreated }: { onCreated?: () => void }) {
  const [open, setOpen] = useState(false);
  const [state, action, pending] = useActionState(async (_: { error?: string }, form: FormData): Promise<{ error?: string }> => {
    const name = String(form.get("name") ?? "").trim();
    if (!name || name.length > 100) return { error: "请输入 1–100 字符的团队名称" };
    try { await apiJSON("/api/teams", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name }) }); setOpen(false); onCreated?.(); return {}; }
    catch { return { error: "创建团队失败，请重试。" }; }
  }, {});

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="lg"><Plus />新建团队</Button>
      </DialogTrigger>
      <DialogContent>
        <form action={action}>
          <DialogHeader>
            <DialogTitle>新建团队</DialogTitle>
            <DialogDescription>创建团队后你将成为团队拥有者。</DialogDescription>
          </DialogHeader>
          <div className="py-5">
            <div className="space-y-2">
              <Label htmlFor="team-name">团队名称</Label>
              <Input autoFocus id="team-name" name="name" placeholder="请输入团队名称" required />
            </div>
            {state.error ? <p className="mt-3 text-sm text-destructive">{state.error}</p> : null}
          </div>
          <DialogFooter>
            <DialogClose asChild><Button type="button" variant="outline">取消</Button></DialogClose>
            <Button disabled={pending} type="submit">创建团队</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
