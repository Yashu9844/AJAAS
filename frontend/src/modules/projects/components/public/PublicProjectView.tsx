import { formatDay } from "../../lib/dates";
import type { PublicProject } from "../../types/project";
import { PriorityChip } from "../ui/Chips";
import type { Priority } from "../../types/common";

const PRIORITIES = ["none", "low", "medium", "high", "urgent"];

/** Read-only columns and tasks of a public project. Everything is rendered as text; there are no links or actions. */
export function PublicProjectView({ data }: { data: PublicProject }) {
  return (
    <div className="pm-board" role="group" aria-label="Project board">
      {data.columns.map((col) => {
        const tasks = data.tasks.filter((t) => t.column === col.name);
        const color = /^#[0-9a-fA-F]{3,8}$/.test(col.color) ? col.color : "#94a3b8";
        return (
          <section key={col.name} className="pm-column" aria-label={`${col.name}, ${tasks.length} tasks shown`}>
            <header className="pm-column-head">
              <span className="pm-dot" style={{ background: color }} aria-hidden="true" />
              <h3>{col.name}</h3>
              <span className="pm-count-badge">{tasks.length}</span>
            </header>
            <ul className="pm-column-body">
              {tasks.length === 0 && <li className="pm-column-empty pm-muted">No tasks</li>}
              {tasks.map((t) => (
                <li key={t.key} className="pm-task-card pm-public-card">
                  <span className="pm-key">{t.key}</span>
                  <span className="pm-task-title">{t.title}</span>
                  <div className="pm-task-meta">
                    {PRIORITIES.includes(t.priority) && <PriorityChip priority={t.priority as Priority} />}
                    {t.due_at && <span className="pm-due">{formatDay(t.due_at)}</span>}
                    {t.progress > 0 && <span className="pm-count">{Math.round(t.progress)}% done</span>}
                    {t.labels.map((l) => (
                      <span key={l} className="pm-chip">
                        {l}
                      </span>
                    ))}
                  </div>
                  {t.description && <p className="pm-muted pm-text">{t.description}</p>}
                </li>
              ))}
            </ul>
          </section>
        );
      })}
      {data.columns.length === 0 && <p className="pm-muted">This project has no columns.</p>}
    </div>
  );
}
