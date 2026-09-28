"use client";

import { useState } from "react";
import { apiJSON, APIError } from "@/lib/api-client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Assignee = Record<string, unknown>;

export function IssueCollaborationPanel({ projectId, issueId, stateVersion, status, labels, assignees, members, agents, canHandle, canAssign, canUseAgent, onChanged, section }: {
  section: "conversation" | "properties";
  projectId: number; issueId: number; stateVersion: number; status: string; labels: string[];
  assignees: Assignee[]; members: Assignee[]; agents: Assignee[]; canHandle: boolean; canAssign: boolean; canUseAgent: boolean;
  onChanged: () => void;
}) {
  const [comment, setComment] = useState("");
  const [labelText, setLabelText] = useState(labels.join(", "));
  const [selected, setSelected] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function mutate(mutation: Record<string, unknown>) {
    setPending(true); setError(null);
    try { await apiJSON(`/api/projects/${projectId}/issues/${issueId}/actions`, {
      method: "POST", headers: { "content-type": "application/json" },
      body: JSON.stringify({ expectedVersion: stateVersion, clientKey: crypto.randomUUID(), mutation })
    });
    setComment(""); onChanged();
    } catch (error) { setError(error instanceof APIError ? error.code : "案件更新失败，请稍后重试。"); }
    finally { setPending(false); }
  }
  const options = [
    ...members.map((item) => ({ value: `user:${String(item.id)}`, label: `${String(item.name)}（成员）` })),
    ...agents.filter((item) => item.kind !== "copilot" || canUseAgent).map((item) => ({
      value: `agent:${String(item.id)}`,
      label: item.kind === "copilot" ? "Copilot（AI）" : `${String(item.name)}（智能体）`
    }))
  ];
  return <div className="space-y-5">
    {section === "properties" && <section className="space-y-2"><h3 className="text-sm font-medium">负责人</h3><div className="flex flex-wrap gap-2">
      {assignees.length ? assignees.map((item) => <Badge key={String(item.id)} variant="outline">{String(item.name)} · {item.assigneeType === "agent" ? "智能体" : "成员"}{canAssign ? <button className="ml-1" disabled={pending} onClick={() => mutate({ action: "unassign", assigneeType: item.assigneeType, assigneeId: Number(item.assigneeId) })} type="button">×</button> : null}</Badge>) : <span className="text-sm text-muted-foreground">尚未指派</span>}
    </div>{canAssign ? <div className="flex gap-2"><select aria-label="选择负责人" className="h-8 min-w-0 flex-1 rounded-lg border bg-background px-2.5 text-sm" onChange={(event) => setSelected(event.target.value)} value={selected}><option value="">选择成员或智能体</option>{options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select><Button disabled={!selected || pending} onClick={() => { const [assigneeType, id] = selected.split(":"); return mutate({ action: "assign", assigneeType, assigneeId: Number(id) }); }} variant="outline">指派</Button></div> : null}</section>}
    {canHandle ? <>
      {section === "conversation" && <section className="space-y-3"><h3 className="text-sm font-medium">添加评论</h3><textarea aria-label="评论内容" className="min-h-36 w-full rounded-md border bg-background p-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" maxLength={5000} onChange={(event) => setComment(event.target.value)} placeholder="记录调查进展，或 @copilot 请求协助…" value={comment} /><div className="flex flex-wrap items-center justify-between gap-3">
      <Button disabled={pending} onClick={() => mutate({ action: "status", status: status === "closed" ? "open" : "closed" })} variant="outline">{status === "closed" ? "重新打开案件" : "关闭案件"}</Button>
      <Button disabled={!comment.trim() || pending} onClick={() => mutate({ action: "comment", body: comment })}>发表评论</Button></div></section>}
      {section === "properties" && <section className="space-y-2 border-t pt-5"><h3 className="text-sm font-medium">标签</h3><div className="flex flex-wrap gap-1.5">{labels.length ? labels.map(label => <Badge key={label} variant="secondary">{label}</Badge>) : <span className="text-sm text-muted-foreground">暂无标签</span>}</div><Input aria-label="案件标签" onChange={(event) => setLabelText(event.target.value)} placeholder="用逗号分隔标签" value={labelText} /><Button size="sm" disabled={pending} onClick={() => mutate({ action: "labels", labels: labelText.split(",") })} variant="outline">保存标签</Button></section>}
    </> : section === "conversation" ? <p className="text-sm text-muted-foreground">你可以查看案件，但没有评论或处置权限。</p> : <section className="space-y-2 border-t pt-5"><h3 className="text-sm font-medium">标签</h3><div className="flex flex-wrap gap-1.5">{labels.length ? labels.map(label => <Badge key={label} variant="secondary">{label}</Badge>) : <span className="text-sm text-muted-foreground">暂无标签</span>}</div></section>}
    {error ? <p className="text-sm text-destructive">{error === "ISSUE_VERSION_CONFLICT" ? "案件已被其他人更新，请刷新后重试。" : error}</p> : null}
  </div>;
}
