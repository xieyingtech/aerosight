import { Badge } from "@/components/ui/badge";
import { DeviceActionPanel } from "@/components/device-action-panel";
import type { DeviceTreeNode } from "@/lib/device-tree-core";

function DeviceNode({ node, projectId, depth = 0 }: { node: DeviceTreeNode; projectId: number; depth?: number }) {
  const available = node.capabilities.filter((capability) => capability.availability === "available");
  return <li className="space-y-2">
    <article className="rounded-xl border bg-card p-4" style={{ marginLeft: `${depth * 20}px` }}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="font-medium">{node.name}</h2>
            <Badge variant="outline">{node.category}</Badge>
            {node.relationType && <span className="text-xs text-muted-foreground">{node.relationType}</span>}
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{node.typeName} · {node.typeKey}</p>
          <p className="text-xs text-muted-foreground">Driver {node.driverKey}@{node.driverVersion}{node.vendor ? ` · ${node.vendor}` : ""}{node.model ? ` · ${node.model}` : ""}</p>
        </div>
        <div className="text-right text-xs">
          <p className={node.status === "online" ? "text-emerald-600" : node.status === "degraded" ? "text-amber-600" : "text-muted-foreground"}>{node.status}</p>
          <p className="text-muted-foreground">{node.dataFreshness}{node.statusReason ? ` · ${node.statusReason}` : ""}</p>
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-1.5">
        {available.map((capability) => <Badge key={capability.code} variant="secondary">{capability.code}</Badge>)}
        {!available.length && <span className="text-xs text-muted-foreground">无可用能力</span>}
      </div>
      {!!node.channels.length && <div className="mt-3 grid gap-1 text-xs text-muted-foreground md:grid-cols-2">
        {node.channels.map((channel) => <div className="rounded-md border px-2 py-1.5" key={channel.stableChannelId}>{channel.name} · {channel.dataType} · {channel.availability}</div>)}
      </div>}
      <DeviceActionPanel actions={available.flatMap((capability) => capability.actions)} deviceId={node.id} projectId={projectId} />
    </article>
    {!!node.children.length && <ul className="space-y-2">{node.children.map((child) => <DeviceNode depth={depth + 1} key={child.id} node={child} projectId={projectId} />)}</ul>}
  </li>;
}

export function DeviceTree({ nodes, projectId }: { nodes: DeviceTreeNode[]; projectId: number }) {
  return nodes.length ? <ul className="space-y-3">{nodes.map((node) => <DeviceNode key={node.id} node={node} projectId={projectId} />)}</ul>
    : <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">暂无已认领设备</div>;
}
