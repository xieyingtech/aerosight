// Resource identity belongs to the path; view options remain in the query.
export const resourcePages: Record<string, { segment: string; key: string }> = {
  "assets/detail": { segment: "assets", key: "assetId" },
  "inspection/summary": { segment: "inspection/runs", key: "runId" },
  "tasks/detail": { segment: "tasks", key: "taskId" },
  "tasks/runs/detail": { segment: "tasks/runs", key: "runId" },
  "algorithms/runs/detail": { segment: "algorithms/runs", key: "runId" },
  "issues/detail": { segment: "issues", key: "issueId" },
  "events/detail": { segment: "events", key: "eventId" },
  "reports/detail": { segment: "reports", key: "reportId" },
  "inspection/observation": { segment: "inspection/observations", key: "observationId" },
  "inspection/evidence": { segment: "inspection/evidence-sets", key: "evidenceSetId" },
  "inspection/assessment": { segment: "inspection/assessments", key: "assessmentId" }
};

export function scopedPageQuery(pathname: string, search: string) {
  const query = new URLSearchParams(search);
  const parts = pathname.split("/").filter(Boolean);
  if (parts[0] === "teams" && /^[1-9]\d*$/.test(parts[1] ?? "")) query.set("teamId", parts[1]);
  if (parts[0] !== "projects" || !/^[1-9]\d*$/.test(parts[1] ?? "")) return query;
  query.set("projectId", parts[1]);
  const segment = parts.slice(2, -1).join("/");
  const resource = Object.values(resourcePages).find(item => item.segment === segment);
  if (resource) query.set(resource.key, parts.at(-1)!);
  if (parts[2] === "realtime" && parts[3] === "devices" && parts[4]) query.set("deviceId", parts[4]);
  return query;
}

export function projectPageHref(projectId: number | string, page: string, parameters: Record<string, string | number> = {}) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(parameters)) if (key !== "projectId") query.set(key, String(value));
  const resource = resourcePages[page];
  let segment = page === "detail" ? "" : page;
  if (resource && query.has(resource.key)) {
    segment = `${resource.segment}/${encodeURIComponent(query.get(resource.key)!)}`;
    query.delete(resource.key);
  }
  if (page === "realtime" && query.has("deviceId")) {
    segment = `realtime/devices/${encodeURIComponent(query.get("deviceId")!)}`;
    query.delete("deviceId");
  }
  const path = `/projects/${projectId}/${segment ? `${segment}/` : ""}`;
  return query.size ? `${path}?${query}` : path;
}

export function canonicalPageHref(href: string) {
  const url = new URL(href, "http://local.invalid");
  const projectId = url.searchParams.get("projectId");
  if (!projectId || !url.pathname.startsWith("/projects/")) return href;
  const page = url.pathname.slice("/projects/".length).replace(/\/$/, "");
  return projectPageHref(projectId, page, Object.fromEntries(url.searchParams)) + url.hash;
}
