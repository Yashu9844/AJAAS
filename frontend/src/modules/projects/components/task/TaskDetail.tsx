"use client";

import Link from "next/link";
import { useTaskDetail } from "../../hooks/use-task-detail";
import { useProjectLoader } from "../../hooks/use-project";
import { projectPath } from "../../lib/paths";
import { ErrorNotice, SkeletonRows } from "../ui/States";
import { StatusPill } from "../ui/Chips";
import { ActivitySection } from "./ActivitySection";
import { AttachmentsSection } from "./AttachmentsSection";
import { CommentsSection } from "./CommentsSection";
import { CustomFieldsSection } from "./CustomFieldsSection";
import { RelationsSection } from "./RelationsSection";
import { SubtasksSection } from "./SubtasksSection";
import { TaskActions } from "./TaskActions";
import { TaskLinksSection } from "./TaskLinksSection";
import { TaskHeaderEditor } from "./TaskHeaderEditor";
import { TaskProperties } from "./TaskProperties";
import { TimeSection } from "./TimeSection";

interface Props {
  idOrKey: string;
  /** Called after any change so the list or board behind a drawer can refresh. */
  onChanged?: () => void;
  showProjectLink?: boolean;
}

/** The whole task: header, properties, subtasks, relations, comments, time, attachments, custom fields, activity. */
export function TaskDetail({ idOrKey, onChanged, showProjectLink }: Props) {
  const detail = useTaskDetail(idOrKey, onChanged);
  const project = useProjectLoader(detail.task?.project_id ?? null);
  const { task } = detail;

  if (detail.error !== undefined && !task) return <ErrorNotice error={detail.error} onRetry={detail.reload} />;
  if (!task || !project.value) {
    if (project.error !== undefined) return <ErrorNotice error={project.error} onRetry={project.value ? undefined : detail.reload} />;
    return <SkeletonRows rows={6} height={40} />;
  }
  const ctx = project.value;
  const reloadAll = () => {
    detail.reload();
    onChanged?.();
  };
  return (
    <article className="pm-task" aria-label={`Task ${task.key}`}>
      {showProjectLink && (
        <p>
          <Link href={projectPath(task.project_id)}>
            {ctx.project.key} · {ctx.project.name}
          </Link>
        </p>
      )}
      <div className="pm-task-flags">
        {task.overdue && <StatusPill kind="overdue">Overdue</StatusPill>}
        {task.blocked && <StatusPill kind="blocked">Blocked</StatusPill>}
        {task.archived_at && <StatusPill kind="warning">Archived</StatusPill>}
      </div>
      <TaskHeaderEditor key={`${task.id}-${task.version}`} task={task} canEdit={ctx.canWrite} onSave={detail.save} />
      <TaskProperties task={task} columns={ctx.columns} labels={ctx.labels} members={ctx.members} canWrite={ctx.canWrite} onSave={detail.save} onReload={reloadAll} />
      <TaskActions task={task} canWrite={ctx.canWrite} onChanged={reloadAll} />
      <CustomFieldsSection task={task} members={ctx.members} canWrite={ctx.canWrite} onReload={reloadAll} />
      <SubtasksSection task={task} canWrite={ctx.canWrite} />
      <TaskLinksSection task={task} />
      <RelationsSection task={task} canWrite={ctx.canWrite} />
      <CommentsSection task={task} members={ctx.members} canComment={ctx.canWrite} />
      <TimeSection task={task} canLog={ctx.canWrite} enabled={ctx.project.time_tracking_enabled} />
      <AttachmentsSection task={task} canUpload={ctx.canWrite} />
      <ActivitySection task={task} columns={ctx.columns} />
    </article>
  );
}
