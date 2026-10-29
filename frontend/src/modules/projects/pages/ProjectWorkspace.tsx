"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { RoleBadge, StatusPill } from "../components/ui/Chips";
import { EmptyState, ErrorNotice, SkeletonRows } from "../components/ui/States";
import { ProjectProvider, useProjectLoader, type ProjectContextValue } from "../hooks/use-project";
import { isProjectId, projectPath, ROUTES, type ProjectTab } from "../lib/paths";

function tabsFor(ctx: ProjectContextValue): { tab: ProjectTab; label: string }[] {
  const tabs: { tab: ProjectTab; label: string }[] = [
    { tab: "board", label: "Board" },
    { tab: "list", label: "List" },
    { tab: "calendar", label: "Calendar" },
    { tab: "gantt", label: "Gantt" },
  ];
  if (ctx.project.mode === "scrum") tabs.push({ tab: "backlog", label: "Backlog & sprints" });
  tabs.push({ tab: "settings", label: "Settings" });
  return tabs;
}

function Tabs({ ctx }: { ctx: ProjectContextValue }) {
  const pathname = usePathname();
  return (
    <nav className="pm-tabs" aria-label="Project views">
      {tabsFor(ctx).map(({ tab, label }) => {
        const href = projectPath(ctx.project.id, tab);
        return (
          <Link key={tab} href={href} className="pm-tab" aria-current={pathname === href ? "page" : undefined}>
            {label}
          </Link>
        );
      })}
    </nav>
  );
}

/** Loads the project once and shares it (with columns, labels, members) with the tab pages below. */
export function ProjectWorkspace({ projectId, children }: { projectId: string; children: ReactNode }) {
  const valid = isProjectId(projectId);
  const load = useProjectLoader(valid ? projectId : null);

  if (!valid) return <EmptyState title="Project not found" action={<Link href={ROUTES.projects}>Back to projects</Link>} />;
  if (load.error !== undefined) return <ErrorNotice error={load.error} />;
  if (!load.value) return <SkeletonRows rows={4} height={48} />;
  const ctx = load.value;
  return (
    <ProjectProvider value={ctx}>
      <div className="pm-workspace">
        <div className="pm-page-head">
          <h1>
            <span className="pm-key">{ctx.project.key}</span> {ctx.project.name}
          </h1>
          <span className="pm-head-badges">
            <RoleBadge role={ctx.project.my_role} />
            {ctx.project.status === "archived" && <StatusPill kind="warning">Archived</StatusPill>}
          </span>
        </div>
        <Tabs ctx={ctx} />
        {children}
      </div>
    </ProjectProvider>
  );
}
