"use client";
import { PM_PROJECTS } from "./generated/projectsSpecs";
import { ContextPage } from "./hrPages";
import { S_PROJECTS } from "./scenariosPm";
import type { ChecklistItem } from "./types";

// M2 Projects & Tasks page of the test console (docs/TEST_CONSOLE.md). Operation forms are generated from the swagger
// contract (scripts/gen_console_specs.py); scenarios are the specification goldens in scenariosPm.ts. Sign in on the
// Identity page first: the scenarios create their own users in session A's tenant.

export const PM_CHECKLIST: ChecklistItem[] = S_PROJECTS.map((s) => ({ label: s.title, keys: [s.key] }));

export function PmProjects() {
  return (
    <ContextPage
      title="M2 Projects & Tasks"
      intro="Projects with owners, members and built-in or custom roles; board columns, labels and milestones; archive and delete. Private projects are invisible to non-members. Creating needs projects:create; administering any project needs projects:manage_all."
      ops={PM_PROJECTS}
      scenarios={S_PROJECTS}
      checklist={PM_CHECKLIST}
    />
  );
}
