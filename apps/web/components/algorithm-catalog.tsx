"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { coerceSchemaParameters } from "@/lib/algorithm-run-input";

type Entry = {
  id: string; versionId: string; version: number; name: string; description: string | null; capabilityCode: string;
  execution: { mode: string; modelOrProcess: string }; provider: { type: string; available: boolean };
  schemas: { parameters: Record<string, unknown>; output: Record<string, unknown> }; display: Record<string, unknown>;
};

function parameterProperties(schema: Record<string, unknown>) {
  return schema.properties && typeof schema.properties === "object" && !Array.isArray(schema.properties)
    ? schema.properties as Record<string, Record<string, unknown>> : {};
}

export function AlgorithmCatalog({ projectId, entries, canRun }: { projectId: number; entries: Entry[]; canRun: boolean }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  async function run(entry: Entry, data: FormData) {
    setError(null);
    const values = Object.fromEntries(data.entries());
    const response = await fetch(`/api/projects/${projectId}/algorithm-runs`, {
      method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({
        definitionVersionId: Number(entry.versionId), assetId: Number(data.get("assetId")),
        parameters: coerceSchemaParameters(entry.schemas.parameters, values)
      })
    });
    const result = await response.json();
    if (!response.ok) setError(result.error ?? "算法运行提交失败");
    else router.push(`/projects/${projectId}/algorithms/runs/${result.runId}`);
  }
  return <div className="grid gap-3 md:grid-cols-2">{entries.map((entry) => <Card key={entry.versionId}>
    <CardHeader><CardTitle>{entry.name}</CardTitle><CardDescription>{entry.capabilityCode} · v{entry.version} · {entry.execution.modelOrProcess}</CardDescription></CardHeader>
    <CardContent className="space-y-3"><p className="text-sm text-muted-foreground">{entry.description ?? "由服务端 schema 描述的通用算法"}</p>
      {canRun ? <form action={(data) => run(entry, data)} className="space-y-2"><Input min="1" name="assetId" placeholder="输入资产 ID" required type="number" />
        {Object.entries(parameterProperties(entry.schemas.parameters)).map(([key, property]) => <label className="grid gap-1 text-xs" key={key}><span>{String(property.title ?? key)}</span>
          {property.type === "boolean" ? <select className="h-9 rounded-md border bg-transparent px-3 text-sm" name={key}><option value="">默认</option><option value="true">是</option><option value="false">否</option></select>
            : <Input name={key} placeholder={String(property.description ?? key)} type={property.type === "number" || property.type === "integer" ? "number" : "text"} />}</label>)}
        <Button disabled={!entry.provider.available} type="submit">运行</Button></form> : null}
    </CardContent></Card>)}{entries.length === 0 ? <p className="text-sm text-muted-foreground">尚无已发布算法定义</p> : null}{error ? <p className="text-sm text-destructive">{error}</p> : null}</div>;
}
