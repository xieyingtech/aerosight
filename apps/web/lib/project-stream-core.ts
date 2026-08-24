export type StreamProjectEvent = {
  cursor: string;
  eventId: string;
  eventType: string;
  payload: Record<string, unknown>;
  occurredAt: string | Date;
};

export function parseEventCursor(value: string | null) {
  if (!value || !/^\d+$/.test(value)) return "0";
  return BigInt(value).toString();
}

export function encodeServerSentEvent(event: StreamProjectEvent) {
  const data = JSON.stringify({
    eventId: event.eventId,
    type: event.eventType,
    payload: event.payload,
    occurredAt: event.occurredAt instanceof Date ? event.occurredAt.toISOString() : event.occurredAt
  });
  return `id: ${event.cursor}\nevent: ${event.eventType}\ndata: ${data}\n\n`;
}

export function encodeSnapshotRequired(cursor: string, projectId: number) {
  return `id: ${cursor}\nevent: snapshot.required\ndata: ${JSON.stringify({
    reason: "replay_window_exceeded",
    snapshotUrl: `/api/projects/${projectId}/snapshot`,
    resumeCursor: cursor
  })}\n\n`;
}

export function replayDecision(events: StreamProjectEvent[], replayLimit: number) {
  if (events.length > replayLimit) {
    return { kind: "snapshot-required" as const, cursor: events.at(-1)?.cursor ?? "0", events: [] };
  }
  return { kind: "events" as const, cursor: events.at(-1)?.cursor, events };
}

export function shouldRecheckAuthorization(lastCheckedAt: number, now: number, intervalMilliseconds: number) {
  return now - lastCheckedAt >= intervalMilliseconds;
}
