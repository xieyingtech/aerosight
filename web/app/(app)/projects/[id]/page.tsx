import { apiFetch, type Project } from "@/lib/api";
import { Page } from "@/components/ui";
import { ProjectMap } from "@/components/project-map";

export default async function ProjectOverview({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const project = await apiFetch<Project>(`/api/projects/${id}`);
  return (
    <Page description={project.description ?? "暂无描述"} title={project.name}>
      <ProjectMap />
    </Page>
  );
}
