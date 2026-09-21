const queryNames: Record<string, string> = {
  query_devices: "设备状态",
  query_tasks: "任务进展",
  query_issues: "案件",
  query_assets: "数据资产",
  query_tracks: "设备轨迹",
  query_map_context: "地图态势",
};

const queryDescriptions: Record<string, string> = {
  query_devices: "查询设备的类型、驱动、运行状态和数据新鲜度。",
  query_tasks: "查询任务列表及最近运行状态，了解任务执行进展。",
  query_issues: "查询案件的状态、优先级和证据质量。",
  query_assets: "查询可用的数据资产及其版本。",
  query_tracks: "查询设备轨迹摘要，了解设备的活动情况。",
  query_map_context: "查询地图态势摘要，汇总项目的空间信息。",
};

type Evidence = { type: string; id: string; version: string; href?: string };
type Query = { name?: string; status?: string; summary?: string; evidenceRefs?: Evidence[] };

export function AgentQueryEvidence({ toolCalls, inline = false }: { toolCalls: unknown; inline?: boolean }) {
  if (!Array.isArray(toolCalls)) return null;
  const queries = toolCalls.filter((item): item is Query => Boolean(item) && typeof item === "object");
  if (!queries.length) return null;
  if (inline) return <div className="my-3 space-y-2">{queries.map((query, index) => <AgentQueryEvidence key={index} toolCalls={[query]} />)}</div>;
  const hasFailure = queries.some(item => item.status === "failed");
  const single = queries.length === 1 ? queries[0] : null;

  return <details className="mt-4 text-xs">
    <summary className="w-fit cursor-pointer text-muted-foreground hover:text-foreground">
      {single ? <><span className={single.status === "running" ? "animate-pulse" : ""}>{queryNames[single.name ?? ""] ?? single.name ?? "项目查询"}</span><code className="ml-2">{single.name}</code><span className="ml-2">{single.status === "running" ? "查询中…" : single.status === "failed" ? "查询失败" : single.summary === "返回 0 条项目内记录" ? "无相关记录" : single.summary || "查询完成"}</span></> : <>工具调用 · {queries.length} 次{hasFailure ? " · 部分查询失败" : ""}</>}
    </summary>
    <div className="mt-3 space-y-4 border-l-2 border-border pl-4">
      {queries.map((item, index) => {
        const summary = item.summary === "返回 0 条项目内记录" ? "未查到相关记录（本次查询结果为空）" : item.summary;
        const status = item.status === "succeeded" ? "查询完成" : item.status === "failed" ? "查询失败" : item.status === "running" ? "查询中" : "状态未知";
        return <div key={index}>
          <p className="font-medium">{index + 1}. {queryNames[item.name ?? ""] ?? item.name ?? "项目数据查询"}<span className="ml-2 font-normal text-muted-foreground">{status}</span></p>
          {item.name && <p className="mt-1 text-muted-foreground">调用工具：<code className="break-all font-mono">{item.name}</code></p>}
          {queryDescriptions[item.name ?? ""] && <p className="mt-1 leading-5 text-muted-foreground">查询内容：{queryDescriptions[item.name ?? ""]}</p>}
          <p className="mt-1 text-muted-foreground">查询范围：当前项目中你有权访问的数据</p>
          {summary && <p className="mt-1 leading-5">返回结果：{summary}</p>}
          {Array.isArray(item.evidenceRefs) && item.evidenceRefs.length > 0 && <p className="mt-2 font-medium">相关依据</p>}
          {Array.isArray(item.evidenceRefs) && item.evidenceRefs.map((ref, refIndex) => <p className="mt-1 break-words text-muted-foreground" key={refIndex}>
            {ref.href?.startsWith("/") && !ref.href.startsWith("//") ? <a className="text-primary underline underline-offset-2" href={ref.href}>{ref.type}:{ref.id}</a> : <span>{ref.type}:{ref.id}</span>} · {ref.version}
          </p>)}
        </div>;
      })}
    </div>
  </details>;
}
