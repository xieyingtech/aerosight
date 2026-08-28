"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Assignee = Record<string, unknown>;

export function IssueCollaborationPanel({ projectId, issueId, stateVersion, status, labels, assignees, members, agents, canHandle, canAssign, canUseAgent }: {
  projectId: number; issueId: number; stateVersion: number; status: string; labels: string[];
  assignees: Assignee[]; members: Assignee[]; agents: Assignee[]; canHandle: boolean; canAssign: boolean; canUseAgent: boolean;
}) {
  const router = useRouter();
  const [comment, setComment] = useState("");
  const [labelText, setLabelText] = useState(labels.join(", "));
  const [selected, setSelected] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function mutate(mutation: Record<string, unknown>) {
    setPending(true); setError(null);
    const response = await fetch(`/api/projects/${projectId}/issues/${issueId}/actions`, {
      method: "POST", headers: { "content-type": "application/json" },
      body: JSON.stringify({ expectedVersion: stateVersion, clientKey: crypto.randomUUID(), mutation })
    });
    const result = await response.json();
    setPending(false);
    if (!response.ok) { setError(result.error ?? "案件更新失败"); return; }
    setComment(""); router.refresh();
  }
  const options = [
    ...members.map((item) => ({ value: `user:${String(item.id)}`, label: `${String(item.name)}（成员）` })),
    ...agents.filter((item) => item.kind !== "copilot" || canUseAgent).map((item) => ({
      value: `agent:${String(item.id)}`,
      label: item.kind === "copilot" ? "Copilot（AI）" : `${String(item.name)}（智能体）`
    }))
  ];
  return <div className="space-y-5">
    <section className="space-y-2"><h3 className="text-sm font-medium">当前指派</h3><div className="flex flex-wrap gap-2">
      {assignees.length ? assignees.map((item) => <Badge key={String(item.id)} variant="outline">{String(item.name)} · {item.assigneeType === "agent" ? "智能体" : "成员"}{canAssign ? <button className="ml-1" disabled={pending} onClick={() => mutate({ action: "unassign", assigneeType: item.assigneeType, assigneeId: Number(item.assigneeId) })} type="button">×</button> : null}</Badge>) : <span className="text-sm text-muted-foreground">尚未指派</span>}
    </div>{canAssign ? <div className="flex gap-2"><select className="h-8 min-w-0 flex-1 rounded-lg border bg-background px-2.5 text-sm" onChange={(event) => setSelected(event.target.value)} value={selected}><option value="">选择成员或智能体</option>{options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select><Button disabled={!selected || pending} onClick={() => { const [assigneeType, id] = selected.split(":"); return mutate({ action: "assign", assigneeType, assigneeId: Number(id) }); }} variant="outline">指派</Button></div> : null}</section>
    {canHandle ? <>
      <section className="space-y-2"><h3 className="text-sm font-medium">添加评论</h3><textarea className="min-h-24 w-full rounded-lg border bg-background p-2.5 text-sm" maxLength={5000} onChange={(event) => setComment(event.target.value)} placeholder="记录调查进展，或 @copilot 请求协助…" value={comment} /><Button disabled={!comment.trim() || pending} onClick={() => mutate({ action: "comment", body: comment })}>发表评论</Button></section>
      <section className="space-y-2"><h3 className="text-sm font-medium">标签</h3><div className="flex gap-2"><Input onChange={(event) => setLabelText(event.target.value)} placeholder="逗号分隔" value={labelText} /><Button disabled={pending} onClick={() => mutate({ action: "labels", labels: labelText.split(",") })} variant="outline">保存标签</Button></div></section>
      <Button disabled={pending} onClick={() => mutate({ action: "status", status: status === "closed" ? "open" : "closed" })} variant="outline">{status === "closed" ? "重新打开案件" : "关闭案件"}</Button>
    </> : <p className="text-sm text-muted-foreground">你可以查看案件，但没有评论或处置权限。</p>}
    {error ? <p className="text-sm text-destructive">{error === "ISSUE_VERSION_CONFLICT" ? "案件已被其他人更新，请刷新后重试。" : error}</p> : null}
  </div>;
}
