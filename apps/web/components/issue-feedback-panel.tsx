"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function IssueFeedbackPanel({ projectId,issueId,stateVersion,detections }: { projectId: number;issueId: number;stateVersion: number;detections: Array<Record<string,unknown>> }) {
  const router = useRouter();
  const [detectionId,setDetectionId] = useState(String(detections[0]?.id ?? ""));
  const [reason,setReason] = useState("");
  const [correctedLabel,setCorrectedLabel] = useState("");
  const [disposition,setDisposition] = useState("resolved");
  const [pending,setPending] = useState(false);
  const [error,setError] = useState("");
  async function submit(action: "confirm"|"false_positive"|"category_correction"|"disposition") {
    setPending(true);setError("");
    const response = await fetch(`/api/projects/${projectId}/issues/${issueId}/feedback`,{ method: "POST",
      headers: { "content-type": "application/json" },body: JSON.stringify({ expectedVersion: stateVersion,clientKey: crypto.randomUUID(),
        detectionId: Number(detectionId),action,reason,correctedLabel: action === "category_correction" ? correctedLabel : undefined,
        disposition: action === "disposition" ? disposition : undefined }) });
    const result = await response.json();setPending(false);
    if (!response.ok) { setError(String(result.error || "反馈保存失败"));return; }
    setReason("");router.refresh();
  }
  return <div className="space-y-3"><div className="grid gap-2 md:grid-cols-4">
    <select className="h-9 rounded-md border bg-background px-2 text-sm" value={detectionId} onChange={(event) => setDetectionId(event.target.value)}>{detections.map((item) => <option value={String(item.id)} key={String(item.id)}>检测 #{String(item.id)} · {String(item.label)}</option>)}</select>
    <Input placeholder="处置原因（必填）" value={reason} onChange={(event) => setReason(event.target.value)} />
    <Input placeholder="修正类别" value={correctedLabel} onChange={(event) => setCorrectedLabel(event.target.value)} />
    <select className="h-9 rounded-md border bg-background px-2 text-sm" value={disposition} onChange={(event) => setDisposition(event.target.value)}><option value="resolved">已解决</option><option value="monitoring">持续观察</option><option value="remediated">已整改</option><option value="accepted_risk">接受风险</option><option value="not_applicable">不适用</option></select>
  </div><div className="flex flex-wrap gap-2"><Button disabled={pending || !reason || !detectionId} onClick={() => submit("confirm")}>确认</Button><Button disabled={pending || !reason || !detectionId} variant="destructive" onClick={() => submit("false_positive")}>误报</Button><Button disabled={pending || !reason || !correctedLabel || !detectionId} variant="outline" onClick={() => submit("category_correction")}>修正类别</Button><Button disabled={pending || !reason || !detectionId} variant="outline" onClick={() => submit("disposition")}>记录处置结果</Button></div>{error ? <p className="text-sm text-destructive">{error}</p> : null}</div>;
}
