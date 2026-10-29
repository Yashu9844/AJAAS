import { redirect } from "next/navigation";
import { projectPath } from "@/modules/projects/lib/paths";

export default async function Page({ params }: { params: Promise<{ projectId: string }> }) {
  const { projectId } = await params;
  redirect(projectPath(projectId, "board"));
}
