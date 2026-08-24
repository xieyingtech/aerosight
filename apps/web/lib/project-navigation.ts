import type { ProjectPermission } from "@/lib/project-permission-policy";

export type ProjectNavigationItem = {
  key: "overview" | "realtime" | "tasks" | "devices" | "events" | "algorithms" | "agents" | "assets" | "settings";
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
  { key: "devices", title: "设备", segment: "devices" },
  { key: "events", title: "告警事件", segment: "events" },
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
