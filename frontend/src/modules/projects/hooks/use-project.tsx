"use client";

import { createContext, useContext, useEffect, useMemo, type ReactNode } from "react";
import { isForbidden } from "../api/errors";
import { getProject, listColumns, listLabels, listMembers } from "../api/projects-api";
import type { Column, Label, Member, Project } from "../types/project";
import { rememberMembers } from "./people-store";
import { useResource } from "./use-resource";
import { useSession } from "./use-session";

interface ProjectBundle {
  project: Project;
  columns: Column[];
  labels: Label[];
  members: Member[];
}

export interface ProjectContextValue extends ProjectBundle {
  /** UI hints only: the API decides. A 403 from any call is shown as such. */
  canWrite: boolean;
  canManage: boolean;
  reload: () => void;
}

function emptyOnForbidden(e: unknown): never[] {
  if (isForbidden(e)) return [];
  throw e;
}

const ProjectContext = createContext<ProjectContextValue | null>(null);

export interface ProjectLoad {
  loading: boolean;
  error: unknown;
  value: ProjectContextValue | null;
}

/** Loads a project with its columns, labels and members in parallel and exposes them to the workspace tabs. */
export function useProjectLoader(projectId: string | null): ProjectLoad {
  const session = useSession();
  const res = useResource<ProjectBundle>(projectId ? `project:${projectId}` : null, async () => {
    if (!projectId) throw new Error("No project");
    const [project, columns, labels, members] = await Promise.all([getProject(projectId), listColumns(projectId), listLabels(projectId).catch(emptyOnForbidden), listMembers(projectId).catch(emptyOnForbidden)]);
    return { project, columns, labels, members };
  });
  const { data, reload } = res;
  useEffect(() => {
    if (data) rememberMembers(data.members);
  }, [data]);
  const isAdmin = session.roles.includes("tenant_admin");
  const value = useMemo<ProjectContextValue | null>(() => {
    if (!data) return null;
    const role = data.project.my_role;
    const archived = data.project.status === "archived";
    return {
      ...data,
      reload,
      canManage: !archived && (isAdmin || role === "owner" || role === "manager"),
      canWrite: !archived && (isAdmin || role === "owner" || role === "manager" || role === "member"),
    };
  }, [data, isAdmin, reload]);
  return { loading: res.loading, error: res.error, value };
}

export function ProjectProvider({ value, children }: { value: ProjectContextValue; children: ReactNode }) {
  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>;
}

export function useProject(): ProjectContextValue {
  const v = useContext(ProjectContext);
  if (!v) throw new Error("useProject must be used inside a project workspace");
  return v;
}

export function useOptionalProject(): ProjectContextValue | null {
  return useContext(ProjectContext);
}
