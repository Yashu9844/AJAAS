import type { ReactNode } from "react";
import { ProjectWorkspace } from "@/modules/projects/pages/ProjectWorkspace";

export default async function ProjectLayout({ children, params }: { children: ReactNode; params: Promise<{ projectId: string }> }) {
  const { projectId } = await params;
  return <ProjectWorkspace projectId={projectId}>{children}</ProjectWorkspace>;
}
