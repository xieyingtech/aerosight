import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";

export type TimelineLaneKey = "devices" | "tasks" | "media" | "algorithms" | "detections" | "issues";
export type TimelineItem = {
  id: string;
  entityId: string;
  lane: TimelineLaneKey;
  label: string;
  timestamp: string;
  count: number;
  status?: string;
};
export type TimelineGap = { from: string; to: string; reason: "no-data" | "data-gap" };
export type TimelineLane = { key: TimelineLaneKey; label: string; items: TimelineItem[]; gaps: TimelineGap[] };
export type TimelineModel = { from: string; to: string; lanes: TimelineLane[] };

const laneLabels: Record<TimelineLaneKey, string> = {
  devices: "设备", tasks: "任务步骤", media: "媒体", algorithms: "算法运行", detections: "检测", issues: "案件"
};

function dateValue(value: unknown) {
  if (value instanceof Date) return value.toISOString();
  if (typeof value === "string" && Number.isFinite(Date.parse(value))) return new Date(value).toISOString();
  return null;
}

function aggregate(items: TimelineItem[], windowMilliseconds: number) {
  const sorted = items.sort((left, right) => Date.parse(left.timestamp) - Date.parse(right.timestamp));
  const result: TimelineItem[] = [];
  for (const item of sorted) {
    const previous = result.at(-1);
    if (previous && previous.entityId === item.entityId && Date.parse(item.timestamp) - Date.parse(previous.timestamp) <= windowMilliseconds) {
      previous.count += item.count;
      previous.timestamp = item.timestamp;
      previous.label = item.label;
      previous.status = item.status;
    } else result.push({ ...item });
  }
  return result;
}

function gaps(items: TimelineItem[], from: string, to: string, gapThresholdMilliseconds: number): TimelineGap[] {
  if (!items.length) return [{ from, to, reason: "no-data" }];
  const values = items.map((item) => Date.parse(item.timestamp));
  const result: TimelineGap[] = [];
  for (let index = 1; index < values.length; index++) {
    if (values[index] - values[index - 1] > gapThresholdMilliseconds) {
      result.push({ from: new Date(values[index - 1]).toISOString(), to: new Date(values[index]).toISOString(), reason: "data-gap" });
    }
  }
  return result;
}

export function buildTimelineModel(snapshot: ProjectSituationSnapshot, options?: {
  from?: string;
  to?: string;
  aggregateWindowMilliseconds?: number;
  gapThresholdMilliseconds?: number;
}): TimelineModel {
  const to = options?.to ?? snapshot.generatedAt;
  const from = options?.from ?? new Date(Date.parse(to) - 60 * 60 * 1000).toISOString();
  const raw: Record<TimelineLaneKey, TimelineItem[]> = {
    devices: [], tasks: [], media: [], algorithms: [], detections: [], issues: []
  };
  for (const device of snapshot.devices) {
    const pose = device.pose as Record<string, unknown> | null;
    const timestamp = dateValue(pose?.capturedAt ?? device.lastSeenAt);
    if (timestamp) raw.devices.push({ id: `device-${device.id}-${timestamp}`, entityId: String(device.id), lane: "devices", label: String(device.name ?? "设备"), timestamp, count: 1, status: String(device.status ?? "unknown") });
  }
  for (const step of snapshot.taskSteps) {
    const timestamp = dateValue(step.occurredAt);
    if (timestamp) raw.tasks.push({ id: `task-step-${step.id}-${timestamp}`, entityId: String(step.id), lane: "tasks", label: String(step.name ?? step.stepKey ?? "任务步骤"), timestamp, count: 1, status: String(step.status ?? "") });
  }
  for (const run of snapshot.algorithmRuns) {
    const timestamp = dateValue(run.occurredAt);
    if (timestamp) raw.algorithms.push({ id: `algorithm-${run.id}-${timestamp}`, entityId: String(run.id), lane: "algorithms", label: String(run.definitionName ?? "算法运行"), timestamp, count: 1, status: String(run.status ?? "") });
  }
  for (const media of snapshot.mediaPoints) {
    const timestamp = dateValue(media.capturedAt ?? media.createdAt);
    if (timestamp) raw.media.push({ id: `media-${media.id}-${timestamp}`, entityId: String(media.deviceId ?? media.id), lane: "media", label: String(media.kind ?? "媒体"), timestamp, count: 1 });
  }
  for (const detection of snapshot.suspectedConstruction) {
    const timestamp = dateValue(detection.capturedAt ?? detection.createdAt);
    if (timestamp) raw.detections.push({ id: `detection-${detection.id}-${timestamp}`, entityId: String(detection.id), lane: "detections", label: String(detection.label ?? "疑似违建"), timestamp, count: 1, status: String(detection.status ?? "open") });
  }
  for (const issue of snapshot.openIssues) {
    const timestamp = dateValue(issue.updatedAt ?? issue.createdAt);
    if (timestamp) raw.issues.push({ id: `issue-${issue.id}-${timestamp}`, entityId: String(issue.id), lane: "issues", label: String(issue.title ?? "案件"), timestamp, count: 1, status: String(issue.status ?? "open") });
  }
  const aggregationWindow = options?.aggregateWindowMilliseconds ?? 2_000;
  const gapThreshold = options?.gapThresholdMilliseconds ?? 5 * 60_000;
  return {
    from,
    to,
    lanes: (Object.keys(laneLabels) as TimelineLaneKey[]).map((key) => {
      const items = aggregate(raw[key], aggregationWindow).filter((item) => item.timestamp >= from && item.timestamp <= to);
      return { key, label: laneLabels[key], items, gaps: gaps(items, from, to, gapThreshold) };
    })
  };
}

export function timelinePosition(timestamp: string, from: string, to: string) {
  const duration = Math.max(1, Date.parse(to) - Date.parse(from));
  return Math.max(0, Math.min(100, ((Date.parse(timestamp) - Date.parse(from)) / duration) * 100));
}
