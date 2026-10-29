import { TaskPage } from "@/modules/projects/pages/TaskPage";

export default async function Page({ params }: { params: Promise<{ ticket: string }> }) {
  const { ticket } = await params;
  return <TaskPage ticket={decodeURIComponent(ticket)} />;
}
