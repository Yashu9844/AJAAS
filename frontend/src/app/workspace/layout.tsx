import type { Metadata } from "next";
import type { ReactNode } from "react";
import "@/modules/projects/styles/base.css";
import "@/modules/projects/styles/board.css";
import "@/modules/projects/styles/task.css";
import "@/modules/projects/styles/views.css";

export const metadata: Metadata = {
  title: "Projects & Tasks",
  description: "Projects, boards and tasks",
};

export default function WorkspaceLayout({ children }: { children: ReactNode }) {
  return <div className="pm-root">{children}</div>;
}
