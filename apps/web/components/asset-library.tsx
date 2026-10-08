"use client";
import { useRef, useState } from "react";
import { apiJSON } from "@/lib/api-client";
import { useAPI } from "@/lib/use-api";
import { assetName, type AlgorithmAsset } from "@/lib/algorithm-workspace";
import { captureVideoFrame, seekVideoFrame } from "@/lib/video-frame";
import { SampledVideoAlgorithm } from "@/components/sampled-video-algorithm";
import { AlgorithmAssetPreview } from "@/components/algorithm-asset-preview";

function OfflineVideo({ projectId, asset }: { projectId: number; asset: AlgorithmAsset }) {
  const access = useAPI<{ url: string }>(`/api/projects/${projectId}/assets/${asset.id}/access?action=play`);
  const video = useRef<HTMLVideoElement>(null);
  const issuedAt = useRef(Date.now());
  return <div className="space-y-3">
    {access.error ? <p role="alert">视频地址获取失败 <button onClick={access.reload}>重试</button></p>
      : <video ref={video} src={access.data?.url} controls preload="metadata" className="max-h-96 w-full rounded bg-slate-950" />}
    <SampledVideoAlgorithm projectId={projectId} videoAssetId={asset.id} capture={async (signal, offset = 0) => {
      const player = video.current;
      if (!player || !Number.isFinite(player.duration)) throw new Error("等待视频加载后再开始分析");
      if (offset >= player.duration) return null;
      // Renew access before a seek can trigger a fresh HTTP range request.
      if (Date.now() - issuedAt.current > 90_000) {
        const renewed = await apiJSON<{ url: string }>(`/api/projects/${projectId}/assets/${asset.id}/access?action=play`, { signal });
        await new Promise<void>((resolve, reject) => {
          const cleanup = () => { clearTimeout(timer); player.removeEventListener("loadedmetadata", loaded); player.removeEventListener("error", failed); signal.removeEventListener("abort", aborted); };
          const loaded = () => { cleanup(); resolve(); }; const failed = () => { cleanup(); reject(new Error("视频重新加载失败")); }; const aborted = () => { cleanup(); reject(signal.reason); };
          const timer = setTimeout(failed, 15_000);
          player.addEventListener("loadedmetadata", loaded, { once: true }); player.addEventListener("error", failed, { once: true }); signal.addEventListener("abort", aborted, { once: true });
          player.src = renewed.url; player.load();
        });
        issuedAt.current = Date.now();
      }
      await seekVideoFrame(player, offset, signal);
      return captureVideoFrame(player);
    }} />
  </div>;
}

export function AssetLibrary({ projectId }: { projectId: number }) {
  const assets = useAPI<AlgorithmAsset[]>(`/api/projects/${projectId}/assets`);
  const [selected, setSelected] = useState<number | null>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const asset = assets.data?.find(a => a.id === selected);
  async function upload(form: FormData) {
    setUploading(true); setError(null);
    try { const result = await apiJSON<{ assetId: number }>(`/api/projects/${projectId}/assets/import`, { method: "POST", body: form }); setSelected(result.assetId); assets.reload(); }
    catch (e) { setError(e instanceof Error ? e.message : "上传失败"); }
    finally { setUploading(false); }
  }
  return <div className="space-y-4">
    <form action={upload} className="flex flex-wrap items-end gap-3 rounded-lg border p-3">
      <label className="space-y-1 text-sm"><span className="block">照片或视频</span><input name="file" type="file" accept="image/jpeg,image/png,video/mp4" required disabled={uploading} /></label>
      <label className="space-y-1 text-sm"><span className="block">来源说明</span><input name="sourceDescription" placeholder="例如：司空媒体库手动下载" className="rounded border bg-background p-2" disabled={uploading} /></label>
      <button type="submit" disabled={uploading} className="rounded border px-3 py-2 text-sm disabled:opacity-50">{uploading ? "正在上传…" : "导入素材"}</button>
      <p className="w-full text-xs text-muted-foreground">图片最大 40 MB，MP4 视频最大 512 MB。导入后可播放或抽帧分析。</p>
    </form>
    {(error || assets.error) && <p role="alert" className="text-sm text-destructive">{error ?? "素材读取失败"}</p>}
    <div className="grid gap-4 md:grid-cols-[240px_1fr]">
      <div className="max-h-[650px] space-y-1 overflow-auto rounded border p-2">{assets.data?.map(a => <button key={a.id} onClick={() => setSelected(a.id)} className={`block w-full rounded p-2 text-left text-sm ${selected === a.id ? "bg-muted" : "hover:bg-muted/50"}`}><span className="block truncate">{assetName(a)}</span><span className="text-xs text-muted-foreground">{a.kind} · {new Date(a.capturedAt ?? a.createdAt).toLocaleString()}</span></button>)}{!assets.loading && !assets.data?.length && <p className="p-4 text-sm text-muted-foreground">暂无素材</p>}</div>
      <div className="space-y-3">{asset?.sourceDescription && <p className="text-xs text-muted-foreground">{asset.sourceDescription}</p>}{asset?.mimeType?.startsWith("video/") ? <OfflineVideo key={asset.id} projectId={projectId} asset={asset} /> : asset?.mimeType?.startsWith("image/") ? <AlgorithmAssetPreview key={asset.id} projectId={projectId} assetId={asset.id} /> : <p className="p-6 text-sm text-muted-foreground">选择素材查看内容</p>}</div>
    </div>
  </div>;
}
