import type { ProjectPermission } from "@/lib/project-permission-policy";

export type ProjectNavigationItem = {
  key: "overview" | "realtime" | "tasks" | "flight-operations" | "geospatial" | "models" | "devices" | "connectors" | "issues" | "algorithms" | "agents" | "assets" | "settings";
  title: string;
  segment: string;
  exact?: boolean;
  permission?: ProjectPermission;
  managementOnly?: boolean;
};

const projectNavigation: ProjectNavigationItem[] = [
  { key: "overview", title: "总览", segment: "", exact: true },
  { key: "realtime", title: "实时作业", segment: "realtime" },
  { key: "tasks", title: "任务", segment: "tasks" },
  { key: "flight-operations", title: "飞行运营", segment: "flight-operations" },
  { key: "geospatial", title: "地图空域", segment: "geospatial" },
  { key: "models", title: "模型重建", segment: "models" },
  { key: "devices", title: "设备", segment: "devices" },
  { key: "connectors", title: "连接器", segment: "connectors", managementOnly: true },
  { key: "issues", title: "案件", segment: "issues" },
  { key: "algorithms", title: "算法服务", segment: "algorithms", permission: "algorithm:manage" },
  { key: "agents", title: "智能体", segment: "agents", permission: "agent:use" },
  { key: "assets", title: "数据资产", segment: "assets" },
  { key: "settings", title: "设置", segment: "settings", managementOnly: true }
];

export function visibleProjectNavigation(
  role: "owner" | "admin" | "member",
  permissions: readonly ProjectPermission[] = []
) {
  const isManager = role === "owner" || role === "admin";
  const granted = new Set(permissions);
  return projectNavigation.filter((item) => {
    if (item.managementOnly) return isManager;
    if (item.permission) return isManager || granted.has(item.permission);
    return true;
  });
}

export function projectNavigationHref(projectId: number, segment: string) {
  const base = `/projects/${projectId}`;
  return segment ? `${base}/${segment}` : base;
}

export function legacyProjectEventListHref(projectId: number) {
  return projectNavigationHref(projectId, "issues");
}
