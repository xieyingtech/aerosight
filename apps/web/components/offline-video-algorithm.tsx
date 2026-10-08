"use client";
import {useRef} from "react";
import {apiJSON} from "@/lib/api-client";
import {useAPI} from "@/lib/use-api";
import type {AlgorithmAsset} from "@/lib/algorithm-workspace";
import {captureVideoFrame, seekVideoFrame} from "@/lib/video-frame";
import {SampledVideoAlgorithm} from "@/components/sampled-video-algorithm";
export function OfflineVideoAlgorithm({ projectId, asset }: { projectId: number; asset: AlgorithmAsset }) {
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

