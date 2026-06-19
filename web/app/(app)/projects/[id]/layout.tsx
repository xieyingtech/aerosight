import Link from "next/link";
import { apiFetch, type Project } from "@/lib/api";

const tabs = [
  ["概览", ""],
  ["设备监控", "devices"],
  ["智能体", "agents"],
  ["任务编排", "tasks"],
  ["问题中心", "issues"],
  ["素材库", "assets"]
] as const;

export default async function ProjectLayout({
  children,
  params
}: {
  children: React.ReactNode;
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const project = await apiFetch<Project>(`/api/projects/${id}`);
  return (
    <div className="grid gap-6 md:grid-cols-[220px_1fr]">
      <aside className="space-y-3">
        <p className="text-sm font-medium text-slate-500">
          {project.teamName}/{project.name}
        </p>
        <nav className="space-y-1">
          {tabs.map(([label, path]) => (
            <Link className="block rounded-md px-3 py-2 text-sm hover:bg-white" href={`/projects/${id}${path ? `/${path}` : ""}`} key={path}>
              {label}
            </Link>
          ))}
        </nav>
      </aside>
      <section>{children}</section>
    </div>
  );
}
