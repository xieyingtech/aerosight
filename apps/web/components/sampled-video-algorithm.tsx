"use client";
import { useEffect, useRef, useState } from "react";
import { apiJSON } from "@/lib/api-client";
import { useAPI } from "@/lib/use-api";
import { runDetections, type Detection } from "@/lib/algorithm-workspace";
import { coerceSchemaParameters } from "@/lib/algorithm-run-input";
import { AlgorithmAssetPreview } from "@/components/algorithm-asset-preview";
import type { CapturedVideoFrame } from "@/lib/video-frame";
import type { AlgorithmCatalogEntry } from "@/lib/web-api-types";

type SampleResult = { runId: string; assetId: number; detections: Detection[]; payload: Record<string, unknown>; capturedAt: string; mediaTimeSeconds: number; latency: number; count: number };

export function SampledVideoAlgorithm({ projectId, streamId, videoAssetId, capture }: {
  projectId: number; streamId?: number; videoAssetId?: number;
  capture: (signal: AbortSignal, offsetSeconds?: number) => Promise<CapturedVideoFrame | null>;
}) {
  const catalog = useAPI<{ definitions: AlgorithmCatalogEntry[] }>(`/api/projects/${projectId}/algorithm-definitions`);
  const entries = catalog.data?.definitions.filter(e => e.provider.available) ?? [];
  const [selected, setSelected] = useState("");
  const entry = entries.find(e => e.configurationSnapshotId === selected) ?? entries[0];
  const [intervalSeconds, setIntervalSeconds] = useState(2);
  const [active, setActive] = useState(false);
  const [status, setStatus] = useState("选择算法后开始识别");
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<SampleResult | null>(null);
  const [history, setHistory] = useState<SampleResult[]>([]);
  const controller = useRef<AbortController | null>(null);
  const parameters = useRef<HTMLFormElement>(null);
  const captureRef = useRef(capture); captureRef.current = capture;
  const stop = () => { controller.current?.abort(); controller.current = null; setActive(false); setStatus("已停止识别"); };
  useEffect(() => () => controller.current?.abort(), [projectId, streamId, videoAssetId]);

  async function start() {
    if (!entry || controller.current) return;
    const worker = new AbortController(); controller.current = worker;
    const signal = worker.signal;
    setActive(true); setError(null); setResult(null); setHistory([]);
    const values = parameters.current ? Object.fromEntries(new FormData(parameters.current).entries()) : {};
    const delay = (ms: number) => new Promise<void>((resolve, reject) => {
      signal.throwIfAborted();
      const abort = () => { clearTimeout(timer); reject(signal.reason); };
      const timer = setTimeout(() => { signal.removeEventListener("abort", abort); resolve(); }, ms);
      signal.addEventListener("abort", abort, { once: true });
    });
    try {
      const params = coerceSchemaParameters(entry.schemas.parameters, values);
      let offset = 0, count = 0;
      while (!signal.aborted) {
        setStatus(videoAssetId ? `正在读取视频 ${offset.toFixed(1)} 秒处` : "正在抽取直播画面");
        const frame = await captureRef.current(signal, videoAssetId ? offset : undefined);
        if (!frame) { setStatus("视频抽帧分析完成"); break; }
        const started = performance.now();
        const form = new FormData(); form.set("file", frame.blob, "frame.jpg");
        // Live timestamps describe the browser's received frame; offline source
        // timestamps are only assigned when the video has a known capture time.
        if (!videoAssetId) form.set("capturedAt", frame.capturedAt);
        form.set("sourceDescription", videoAssetId ? `视频 #${videoAssetId} · ${frame.mediaTimeSeconds.toFixed(3)} 秒` : `直播 #${streamId} 的浏览器抽帧`);
        if (streamId) form.set("streamId", String(streamId));
        if (videoAssetId) { form.set("videoAssetId", String(videoAssetId)); form.set("mediaTimeSeconds", String(frame.mediaTimeSeconds)); }
        const asset = await apiJSON<{ assetId: number }>(`/api/projects/${projectId}/assets/import`, { method: "POST", body: form, signal });
        const submitted = await apiJSON<{ runId: string }>(`/api/projects/${projectId}/algorithm-runs`, { method: "POST", headers: { "content-type": "application/json" }, signal,
          body: JSON.stringify({ configurationSnapshotId: Number(entry.configurationSnapshotId), assetId: asset.assetId, parameters: params }) });
        setStatus("正在等待算法结果");
        const deadline = Date.now() + 10 * 60_000;
        let run: { status: string; canonicalResult: Record<string, unknown>; errorCode?: string };
        for (;;) {
          const detail = await apiJSON<{ run: typeof run }>(`/api/projects/${projectId}/algorithm-runs/${submitted.runId}`, { signal }); run = detail.run;
          if (["succeeded", "failed", "timed_out", "cancelled"].includes(run.status)) break;
          if (Date.now() > deadline) throw new Error("本帧等待超过十分钟，请在算法运行记录中检查");
          await delay(750);
        }
        if (run.status !== "succeeded") throw new Error(run.errorCode ?? `算法执行${run.status}`);
        signal.throwIfAborted(); count++;
        const latency = performance.now() - started;
        const completed: SampleResult = { runId: submitted.runId, assetId: asset.assetId, detections: runDetections(run.canonicalResult), payload: run.canonicalResult.result as Record<string, unknown> ?? {}, capturedAt: frame.capturedAt, mediaTimeSeconds: frame.mediaTimeSeconds, latency, count };
        setResult(completed);
        setHistory(previous => [...previous.slice(-199), completed]);
        setStatus(videoAssetId ? `已分析 ${count} 帧` : "识别完成，等待下一帧");
        if (videoAssetId) { offset += intervalSeconds; }
        else await delay(Math.max(100, intervalSeconds * 1000 - latency));
      }
    } catch (e) {
      if (!signal.aborted) { setError(e instanceof Error ? e.message : "识别失败"); setStatus("识别已暂停"); }
    } finally {
      if (controller.current === worker) { controller.current = null; setActive(false); }
    }
  }
  const properties = entry?.schemas.parameters.properties as Record<string, Record<string, unknown>> | undefined;
  return <section className="space-y-3 rounded-md border p-3">
    <div className="flex flex-wrap items-center gap-2 text-xs">
      <select aria-label="抽帧算法" className="max-w-52 rounded border bg-background p-1.5" disabled={active} value={entry?.configurationSnapshotId ?? ""} onChange={e => setSelected(e.target.value)}>
        {!entries.length && <option value="">暂无可用算法</option>}{entries.map(e => <option key={e.id} value={e.configurationSnapshotId}>{e.name}</option>)}
      </select>
      <label>间隔 <input aria-label="抽帧间隔秒数" className="w-14 rounded border bg-background p-1" type="number" min={1} max={60} value={intervalSeconds} disabled={active} onChange={e => setIntervalSeconds(Math.max(1, Math.min(60, Number(e.target.value) || 2)))} /> 秒</label>
      <button className="rounded border px-2 py-1.5 disabled:opacity-50" type="button" disabled={!entry} onClick={active ? stop : () => void start()}>{active ? "停止识别" : videoAssetId ? "分析视频" : "开始直播识别"}</button>
    </div>
    <form ref={parameters} className="flex flex-wrap gap-2">{Object.entries(properties ?? {}).map(([key, property]) => <label className="text-xs" key={`${entry?.id}-${key}`}>{String(property.title ?? key)} <input name={key} className="w-24 rounded border bg-background p-1" disabled={active} defaultValue={String(property.default ?? "")} placeholder="默认值" type={property.type === "number" || property.type === "integer" ? "number" : "text"} step="any" /></label>)}</form>
    <p className="text-xs text-muted-foreground">{status}{history.length ? ` · ${history.at(-1)!.count} 帧 · 最近一次 ${(history.at(-1)!.latency / 1000).toFixed(2)} 秒` : ""}</p>
    {(error || catalog.error) && <p className="text-xs text-destructive" role="alert">{error ?? "算法列表读取失败"}</p>}
    {!!history.length && <div className="space-y-1">
      <p className="text-xs text-muted-foreground">已识别帧 · 点击回看{history.length === 200 ? "（显示最近 200 帧）" : ""}</p>
      <div className="flex max-h-28 flex-wrap gap-1.5 overflow-auto">{history.map(frame => <button type="button" key={frame.runId} aria-pressed={result?.runId === frame.runId} className={`rounded border px-2 py-1 text-xs ${result?.runId === frame.runId ? "bg-primary text-primary-foreground" : ""}`} onClick={() => setResult(frame)}>{videoAssetId ? `${frame.mediaTimeSeconds.toFixed(1)} 秒` : new Date(frame.capturedAt).toLocaleTimeString()} · {frame.detections.length} 个目标</button>)}</div>
    </div>}
    {result && <div className="space-y-2">
      <p className="text-xs text-muted-foreground">{videoAssetId ? `视频 ${result.mediaTimeSeconds.toFixed(2)} 秒处` : `抽帧时间 ${new Date(result.capturedAt).toLocaleTimeString()}`} · {result.detections.length} 个目标</p>
      <AlgorithmAssetPreview key={result.runId} projectId={projectId} assetId={result.assetId} runId={result.runId} detections={result.detections} />
      <p className="text-xs">{result.detections.map(d => `${d.label} ${(d.confidence * 100).toFixed(0)}%`).join(" · ")}</p>
      {result.payload.kind !== "detection" && !Array.isArray(result.payload.detections) && <pre className="max-h-48 overflow-auto whitespace-pre-wrap text-xs">{JSON.stringify(result.payload, null, 2)}</pre>}
      <a className="text-xs underline" href={`/projects/algorithms/runs/detail/?projectId=${projectId}&runId=${result.runId}`}>查看本帧运行详情</a>
    </div>}
    <p className="text-xs text-muted-foreground">{videoAssetId ? "逐帧等待识别完成后再读取下一个时间点，结果保存在算法运行记录。" : "显示最近已识别帧；处理完成后再抽下一帧，识别速度随算法耗时调整。"}</p>
  </section>;
}
