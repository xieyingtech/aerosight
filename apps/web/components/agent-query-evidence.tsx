const queryNames: Record<string, string> = {
  load_skill: "加载行业技能",
  query_objects: "影像目标查询",
  query_devices: "设备状态",
  query_tasks: "任务进展",
  query_issues: "案件",
  query_assets: "数据资产",
  query_tracks: "设备轨迹",
  query_map_context: "地图态势",
  mutate_issue: "案件操作",
  create_task_draft: "任务草稿",
  query_inspection: "巡检资源",
  sync_flight_resources: "同步飞行和媒体",
  create_inspection_task: "新建巡检任务",
  save_task_draft: "保存任务草稿",
  publish_task: "发布任务",
  set_task_state: "任务启停",
  run_task: "启动任务",
  control_task_run: "任务运行控制",
  submit_flight: "提交真实飞行",
  launch_flight: "执行航线飞行",
  control_flight: "飞行控制",
  run_algorithm: "图片算法识别",
  review_inspection: "巡检人工复核",
  generate_report: "生成报告草稿",
};
export const agentToolLabel = (name: string) => queryNames[name] ?? name;
const isWriteTool = (name?: string) => Boolean(name && name !== "load_skill" && name !== "query_inspection" && !name.startsWith("query_") && queryNames[name]);

const queryDescriptions: Record<string, string> = {
  load_skill: "加载巡检目标查询、证据检查和复核规则，供智能体执行。",
  query_objects: "按需求筛选实际检测目标，并提供可重开的原图与目标框。",
  query_devices: "查询设备的类型、驱动、运行状态和数据新鲜度。",
  query_tasks: "查询任务列表及最近运行状态，了解任务执行进展。",
  query_issues: "查询案件的状态、优先级和证据质量。",
  query_assets: "查询可用的数据资产及其版本。",
  query_tracks: "查询设备轨迹摘要，了解设备的活动情况。",
  query_map_context: "查询地图态势摘要，汇总项目的空间信息。",
};

type Evidence = { type: string; id: string; version: string; href?: string };
type Query = { name?: string; status?: string; summary?: string; evidenceRefs?: Evidence[] };

function statusText(query: Query) {
  if (query.status === "running") return isWriteTool(query.name) ? "准备授权请求…" : "查询中…";
  if (query.status === "executing") return "处理中／待核对";
  if (query.status === "failed") return "执行失败";
  if (query.status === "confirmation_required") return "待授权";
  if (query.status === "succeeded") return isWriteTool(query.name) ? "已处理" : "查询完成";
  if (query.status === "rejected") return "已拒绝";
  if (query.status === "expired") return "已过期";
  return query.summary === "返回 0 条项目内记录" ? "无相关记录" : query.summary || "查询完成";
}

export function AgentQueryEvidence({ toolCalls, inline = false }: { toolCalls: unknown; inline?: boolean }) {
  if (!Array.isArray(toolCalls)) return null;
  const queries = toolCalls.filter((item): item is Query => Boolean(item) && typeof item === "object");
  if (!queries.length) return null;
  if (inline) return <div className="my-3 space-y-2">{queries.map((query, index) => <AgentQueryEvidence key={index} toolCalls={[query]} />)}</div>;
  const hasFailure = queries.some(item => item.status === "failed");
  const single = queries.length === 1 ? queries[0] : null;

  return <details className="mt-4 text-xs">
    <summary className="w-fit cursor-pointer text-muted-foreground hover:text-foreground">
      {single ? <><span className={single.status === "running" ? "animate-pulse" : ""}>{queryNames[single.name ?? ""] ?? single.name ?? "项目查询"}</span><code className="ml-2">{single.name}</code><span className="ml-2">{statusText(single)}</span></> : <>工具调用 · {queries.length} 次{hasFailure ? " · 部分查询失败" : ""}</>}
    </summary>
    <div className="mt-3 space-y-4 border-l-2 border-border pl-4">
      {queries.map((item, index) => {
        const summary = item.summary === "返回 0 条项目内记录" ? "未查到相关记录（本次查询结果为空）" : item.summary;
        const status = statusText(item);
        return <div key={index}>
          <p className="font-medium">{index + 1}. {queryNames[item.name ?? ""] ?? item.name ?? "项目数据查询"}<span className="ml-2 font-normal text-muted-foreground">{status}</span></p>
          {item.name && <p className="mt-1 text-muted-foreground">调用工具：<code className="break-all font-mono">{item.name}</code></p>}
          {queryDescriptions[item.name ?? ""] && <p className="mt-1 leading-5 text-muted-foreground">查询内容：{queryDescriptions[item.name ?? ""]}</p>}
          {!isWriteTool(item.name) && <p className="mt-1 text-muted-foreground">查询范围：当前项目中你有权访问的数据</p>}
          {summary && <p className="mt-1 leading-5">{isWriteTool(item.name) ? "操作内容" : "返回结果"}：{summary}</p>}
          {Array.isArray(item.evidenceRefs) && item.evidenceRefs.length > 0 && <p className="mt-2 font-medium">相关依据</p>}
          {Array.isArray(item.evidenceRefs) && item.evidenceRefs.map((ref, refIndex) => <p className="mt-1 break-words text-muted-foreground" key={refIndex}>
            {ref.href?.startsWith("/") && !ref.href.startsWith("//") ? <a className="text-primary underline underline-offset-2" href={ref.href}>{item.name === 'query_objects' ? '查看目标框与筛选结果' : `${ref.type}:${ref.id}`}</a> : <span>{ref.type}:{ref.id}</span>} · {ref.version}
          </p>)}
        </div>;
      })}
    </div>
  </details>;
}
