type MediaItem = { id: string; status: string; updatedAt: string; summary: Record<string, unknown> };
type LiveMedia = { channels: Record<string, unknown>[]; sessions: Record<string, unknown>[];
  recordings: MediaItem[]; shares: MediaItem[]; converters: MediaItem[] };

function Catalog({ title, items }: { title: string; items: MediaItem[] }) {
  return <section className="rounded-xl border bg-card p-4"><h2 className="font-medium">{title}</h2>
    <div className="mt-3 space-y-2">{items.length ? items.slice(0, 8).map((item) => <div className="rounded-md border p-2 text-xs" key={item.id}>
      <div className="flex justify-between gap-2"><span>#{item.id}</span><span>{item.status}</span></div>
      <p className="mt-1 text-muted-foreground">{Object.entries(item.summary).map(([key,value]) => `${key}: ${String(value)}`).join(" · ") || "无公开摘要"}</p>
    </div>) : <p className="text-xs text-muted-foreground">当前没有资源，上游空列表不影响连接器健康。</p>}</div></section>;
}

export function DJIFlightHubLiveMediaPanel({ media }: { media: LiveMedia }) {
  return <div className="space-y-3">
    <section className="rounded-xl border bg-card p-4"><h2 className="font-medium">司空实时媒体</h2>
      <p className="mt-1 text-xs text-muted-foreground">{media.channels.length} 个视频通道 · {media.sessions.length} 个会话；播放器按上游 supplier/protocol 动态获取短期授权。</p>
      <div className="mt-3 flex flex-wrap gap-2 text-xs">{media.sessions.slice(0,8).map((session) => <span className="rounded-full border px-2 py-1" key={String(session.id)}>{String(session.deviceName)} · {String(session.status)} · {String(session.supplier ?? "待分配")}</span>)}</div>
    </section>
    <div className="grid gap-3 lg:grid-cols-3"><Catalog items={media.recordings} title="录制任务" /><Catalog items={media.shares} title="直播分享" /><Catalog items={media.converters} title="码流转换器" /></div>
  </div>;
}
