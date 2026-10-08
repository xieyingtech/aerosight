export type CapturedVideoFrame = { blob: Blob; width: number; height: number; capturedAt: string; mediaTimeSeconds: number };

// Capture decoded pixels rather than a screenshot of the player controls.
export async function captureVideoFrame(video: HTMLVideoElement): Promise<CapturedVideoFrame> {
  if (video.readyState < 2 || !video.videoWidth || !video.videoHeight) throw new Error("视频尚未解码，等待画面后重试");
  const scale = Math.min(1, 1280 / video.videoWidth, 1280 / video.videoHeight);
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(video.videoWidth * scale));
  canvas.height = Math.max(1, Math.round(video.videoHeight * scale));
  const context = canvas.getContext("2d");
  if (!context) throw new Error("浏览器不支持视频抽帧");
  const capturedAt = new Date().toISOString();
  const mediaTimeSeconds = video.currentTime;
  try { context.drawImage(video, 0, 0, canvas.width, canvas.height); }
  catch { throw new Error("无法读取视频画面，请检查播放源的跨域权限"); }
  const blob = await new Promise<Blob>((resolve, reject) => {
    try { canvas.toBlob(value => value ? resolve(value) : reject(new Error("视频抽帧失败")), "image/jpeg", 0.85); }
    catch { reject(new Error("无法读取视频画面，请检查播放源的跨域权限")); }
  });
  return { blob, width: canvas.width, height: canvas.height, capturedAt, mediaTimeSeconds };
}

export async function seekVideoFrame(video: HTMLVideoElement, seconds: number, signal: AbortSignal) {
  signal.throwIfAborted();
  video.pause();
  if (Math.abs(video.currentTime - seconds) < 0.001 && video.readyState >= 2) return;
  await new Promise<void>((resolve, reject) => {
    const cleanup = () => { clearTimeout(timer); video.removeEventListener("seeked", done); video.removeEventListener("error", failed); signal.removeEventListener("abort", abort); };
    const done = () => { cleanup(); resolve(); };
    const failed = () => { cleanup(); reject(new Error("视频定位失败")); };
    const abort = () => { cleanup(); reject(signal.reason); };
    const timer = setTimeout(failed, 15_000);
    video.addEventListener("seeked", done, { once: true }); video.addEventListener("error", failed, { once: true }); signal.addEventListener("abort", abort, { once: true });
    video.currentTime = seconds;
  });
}
