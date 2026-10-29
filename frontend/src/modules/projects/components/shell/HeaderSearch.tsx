"use client";

import Link from "next/link";
import { useState } from "react";
import { describeError } from "../../api/errors";
import { searchTasks } from "../../api/planning-api";
import { useDebouncedValue } from "../../hooks/use-debounced-value";
import { useResource } from "../../hooks/use-resource";
import { taskPath } from "../../lib/paths";

const MIN_CHARS = 2;

/** Global task search (GET /search) with a debounced query and results linking to the task page. */
export function HeaderSearch() {
  const [text, setText] = useState("");
  const [open, setOpen] = useState(false);
  const query = useDebouncedValue(text.trim(), 300);
  const active = query.length >= MIN_CHARS;
  const res = useResource(active ? `search:${query}` : null, (signal) => searchTasks(query, signal));
  const showPanel = open && text.trim().length >= MIN_CHARS;

  return (
    <div
      className="pm-search"
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) setOpen(false);
      }}
    >
      <label className="pm-sr-only" htmlFor="pm-global-search">
        Search tasks
      </label>
      <input
        id="pm-global-search"
        type="search"
        placeholder="Search tasks or KEY-12"
        autoComplete="off"
        value={text}
        maxLength={120}
        onChange={(e) => {
          setText(e.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={(e) => {
          if (e.key === "Escape") setOpen(false);
        }}
      />
      {showPanel && (
        <div className="pm-search-panel" role="region" aria-label="Search results" aria-live="polite">
          {res.error !== undefined && <p className="pm-muted">{describeError(res.error)}</p>}
          {res.loading && <p className="pm-muted">Searching…</p>}
          {!res.loading && res.error === undefined && res.data?.length === 0 && <p className="pm-muted">No tasks found.</p>}
          <ul>
            {(res.data ?? []).map((hit) => (
              <li key={hit.id}>
                <Link href={taskPath(hit.ticket)} onClick={() => setOpen(false)}>
                  <span className="pm-key">{hit.ticket}</span> <span>{hit.title}</span>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
