"use client";
import { PM_AIHOOKS, PM_AUTOMATION, PM_COLLAB, PM_FIELDS, PM_PLANNING, PM_PLATFORM, PM_PROJECTS, PM_TASKS, PM_WEBHOOKS } from "./generated/projectsSpecs";
import { ContextPage } from "./hrPages";
import type { Scenario } from "./runner";
import { S_PROJECTS } from "./scenariosPm";
import { S_PM_AIHOOKS } from "./scenariosPmAiHooks";
import { S_PM_AUTOMATION } from "./scenariosPmAutomation";
import { S_PM_COLLAB } from "./scenariosPmCollab";
import { S_PM_FIELDS } from "./scenariosPmFields";
import { S_PM_PLANNING } from "./scenariosPmPlanning";
import { S_PM_PLATFORM } from "./scenariosPmPlatform";
import { S_PM_TASKS } from "./scenariosPmTasks";
import { S_PM_WEBHOOKS } from "./scenariosPmWebhooks";
import type { ChecklistItem } from "./types";

// M2 Projects & Tasks pages of the test console (docs/TEST_CONSOLE.md). Operation forms are generated from the swagger
// contract (scripts/gen_console_specs.py); scenarios are the specification goldens in scenariosPm*.ts. Sign in on the
// Identity page first: the scenarios create their own users in session A's tenant.

const items = (list: Scenario[]): ChecklistItem[] => list.map((s) => ({ label: s.title, keys: [s.key] }));

export const PM_CHECKLISTS: Record<string, ChecklistItem[]> = {
  projects: items(S_PROJECTS),
  tasks: items(S_PM_TASKS),
  collab: items(S_PM_COLLAB),
  planning: items(S_PM_PLANNING),
  platform: items(S_PM_PLATFORM),
  fields: items(S_PM_FIELDS),
  webhooks: items(S_PM_WEBHOOKS),
  automation: items(S_PM_AUTOMATION),
  aihooks: items(S_PM_AIHOOKS),
};

/** Every M2 checklist item, for the overview page. */
export const PM_CHECKLIST: ChecklistItem[] = Object.values(PM_CHECKLISTS).flat();

export function PmProjects() {
  return (
    <ContextPage
      title="M2 Projects"
      intro="Projects with owners, members and built-in or custom roles; board columns, labels and milestones; archive and delete. Private projects are invisible to non-members. Creating needs projects:create; administering any project needs projects:manage_all."
      ops={PM_PROJECTS}
      scenarios={S_PROJECTS}
      checklist={PM_CHECKLISTS.projects}
    />
  );
}

export function PmTasks() {
  return (
    <ContextPage
      title="M2 Tasks"
      intro="Tasks with gapless ticket numbers, optimistic versions, ranks, columns and WIP limits, subtasks, dependencies with cycle checks, bulk updates, moves between projects and an append-only activity history."
      ops={PM_TASKS}
      scenarios={S_PM_TASKS}
      checklist={PM_CHECKLISTS.tasks}
    />
  );
}

export function PmCollab() {
  return (
    <ContextPage
      title="M2 Collaboration"
      intro="Comments with mentions and tombstones, watchers, time tracking with one running timer per user, and attachments stored in S3-compatible object storage through presigned URLs."
      ops={PM_COLLAB}
      scenarios={S_PM_COLLAB}
      checklist={PM_CHECKLISTS.collab}
    />
  );
}

export function PmPlanning() {
  return (
    <ContextPage
      title="M2 Planning & Views"
      intro="Sprints (one active per project), backlog and planning moves, board, calendar and Gantt views, My work, full-text search, saved views and revocable calendar-feed tokens."
      ops={PM_PLANNING}
      scenarios={S_PM_PLANNING}
      checklist={PM_CHECKLISTS.planning}
    />
  );
}

export function PmPlatform() {
  return (
    <ContextPage
      title="M2 Group rules & lifecycle"
      intro="Membership rules that follow HR data (department, team, a manager's reports) and the effect of deactivating a login on project membership."
      ops={PM_PLATFORM}
      scenarios={S_PM_PLATFORM}
      checklist={PM_CHECKLISTS.platform}
    />
  );
}

export function PmFields() {
  return (
    <ContextPage
      title="M2 Custom fields"
      intro="Typed, project-scoped custom fields (text, number, checkbox, date, https URL, select, multiselect, project member) and their values on tasks. Defining fields needs project.update; setting values needs task.update and moves the task version."
      ops={PM_FIELDS}
      scenarios={S_PM_FIELDS}
      checklist={PM_CHECKLISTS.fields}
    />
  );
}

export function PmWebhooks() {
  return (
    <ContextPage
      title="M2 Webhooks & Git integrations"
      intro="Outgoing webhooks (generic JSON, Slack, Discord) with signed deliveries, retries and an SSRF-guarded sender, and GitHub/Gitea integrations that link pull requests to tasks by ticket key. Managing needs project.update; secrets are shown once."
      ops={PM_WEBHOOKS}
      scenarios={S_PM_WEBHOOKS}
      checklist={PM_CHECKLISTS.webhooks}
    />
  );
}

export function PmAutomation() {
  return (
    <ContextPage
      title="M2 Automation, recurrence & public links"
      intro="Rules (trigger, conditions, actions) that run as their creator, recurring tasks, and anonymous read-only public project pages with revocable tokens. Rules fire a few seconds after the event."
      ops={PM_AUTOMATION}
      scenarios={S_PM_AUTOMATION}
      checklist={PM_CHECKLISTS.automation}
    />
  );
}

export function PmAiHooks() {
  return (
    <ContextPage
      title="M2 AI hooks"
      intro="Read-only seams for a separate AI module: the tool catalogue (filtered by what the caller may do), compact context packs, the document feed and readable projects. No model lives here."
      ops={PM_AIHOOKS}
      scenarios={S_PM_AIHOOKS}
      checklist={PM_CHECKLISTS.aihooks}
    />
  );
}
