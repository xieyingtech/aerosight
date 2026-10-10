import { AlertTriangleIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { diagnosticPresentation, type OperationDiagnostic } from "@/lib/operation-diagnostics-core";

export function OperationDiagnostics({ items, compact = false }: { items: OperationDiagnostic[]; compact?: boolean }) {
  if (!items.length) return null;
  return <section className="rounded-xl border bg-card p-4">
    <div className="mb-3 flex items-center gap-2"><AlertTriangleIcon className="size-4 text-amber-600" /><h2 className="font-medium">运行诊断</h2></div>
    <div className={compact ? "grid gap-2" : "grid gap-2 md:grid-cols-2"}>
      {items.map((raw) => {
        const item = diagnosticPresentation(raw);
        return <article className="rounded-lg border p-3" key={item.id}>
          <div className="flex flex-wrap items-center justify-between gap-2"><span className="text-sm font-medium">{item.title}</span><span className="flex flex-wrap gap-2"><span>{item.label}</span><Badge variant="outline">{item.status}</Badge></span></div>
          <p className="mt-1 break-all text-xs text-muted-foreground">{item.reason}</p>
          {item.occurredAt && <p className="mt-2 text-xs text-muted-foreground">{new Date(item.occurredAt).toLocaleString("zh-CN")}</p>}
        </article>;
      })}
    </div>
  </section>;
}
