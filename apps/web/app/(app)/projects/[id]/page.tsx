import { getProject } from "@/lib/data";
import { Page } from "@/components/page";
import { ProjectMap } from "@/components/project-map";

export default async function ProjectOverview({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const project = await getProject(Number(id));
  return (
    <Page description={project.description ?? "暂无描述"} title={project.name}>
      <ProjectMap />
    </Page>
  );
}
