export type RealtimeChannel = {
  stableChannelId: string;
  channelKey: string;
  displayName: string;
  dataType: "video" | "audio" | "telemetry" | "sensor" | "events";
  schema: Record<string, unknown>;
  unit: string | null;
  quality: Record<string, unknown>;
  availability: "available" | "degraded" | "unavailable";
};

export function buildRealtimeChannelView(channel: RealtimeChannel) {
  if (!channel.stableChannelId || !channel.channelKey) throw new Error("REALTIME_CHANNEL_ID_REQUIRED");
  const renderer = channel.dataType === "video" || channel.dataType === "audio" ? "media"
    : channel.dataType === "events" ? "event-log" : "time-series";
  const properties = channel.schema.properties;
  const fields = properties && typeof properties === "object" && !Array.isArray(properties)
    ? Object.keys(properties as Record<string, unknown>) : [];
  return {
    id: channel.stableChannelId,
    title: channel.displayName || channel.channelKey,
    renderer,
    fields,
    unit: channel.unit,
    quality: channel.quality,
    available: channel.availability === "available"
  };
}
