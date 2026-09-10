// Match canonical positive PostgreSQL integer IDs, including the int32 upper bound.
const integer = "(?:[1-9][0-9]{0,8}|1[0-9]{9}|20[0-9]{8}|21[0-3][0-9]{7}|214[0-6][0-9]{6}|2147[0-3][0-9]{5}|21474[0-7][0-9]{4}|214748[0-2][0-9]{3}|2147483[0-5][0-9]{2}|21474836[0-3][0-9]|214748364[0-7])";
const uuid = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";
const project = `/projects/:id(${integer})`;

export function legacyPageRedirects() {
  const rules = [
    { source: `/teams/:id(${integer})`, destination: "/teams/detail/?teamId=:id" },
    { source: project, destination: "/projects/detail/?projectId=:id" },
    ...["tasks", "settings", "realtime", "assets", "issues", "events", "devices", "algorithms", "connectors", "agents", "flight-operations", "geospatial", "models"].map(section => ({source: `${project}/${section}`, destination: `/projects/${section}/?projectId=:id`})),
    { source: `${project}/tasks/:taskId(${integer})`, destination: "/projects/tasks/detail/?projectId=:id&taskId=:taskId" },
    { source: `${project}/tasks/runs/:runId(${integer})`, destination: "/projects/tasks/runs/detail/?projectId=:id&runId=:runId" },
    { source: `${project}/algorithms/runs/:runId(${uuid})`, destination: "/projects/algorithms/runs/detail/?projectId=:id&runId=:runId" },
    { source: `${project}/issues/:issueId(${integer})`, destination: "/projects/issues/detail/?projectId=:id&issueId=:issueId" },
    { source: `${project}/events/:eventId(${uuid})`, destination: "/projects/events/detail/?projectId=:id&eventId=:eventId" }
  ];
  return rules.map(rule => ({...rule, permanent: false}));
}
