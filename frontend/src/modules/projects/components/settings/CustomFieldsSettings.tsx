"use client";

import { useState, type FormEvent } from "react";
import { createFieldDefinition, deleteFieldDefinition, listFieldDefinitions, updateFieldDefinition } from "../../api/settings-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { parseOptions } from "../../lib/automation-form";
import { humanize } from "../../lib/format";
import type { CustomFieldDefinition, CustomFieldType } from "../../types/task";
import { EmptyState } from "../ui/States";
import { SectionFrame } from "./SectionState";

const TYPES: CustomFieldType[] = ["text", "number", "checkbox", "date", "url", "select", "multiselect", "user"];
const hasOptions = (t: CustomFieldType) => t === "select" || t === "multiselect";

function FieldRow({ def, editable, onSave, onDelete }: { def: CustomFieldDefinition; editable: boolean; onSave: (name: string, options: string[]) => void; onDelete: () => void }) {
  const [editing, setEditing] = useState(false);
  if (editing) {
    return (
      <li className="pm-card">
        <form
          className="pm-inline-form"
          onSubmit={(e) => {
            e.preventDefault();
            const data = new FormData(e.currentTarget);
            onSave(String(data.get("name") ?? "").trim(), parseOptions(String(data.get("options") ?? "")));
            setEditing(false);
          }}
        >
          <label className="pm-sr-only" htmlFor={`cfn-${def.id}`}>
            Field name
          </label>
          <input id={`cfn-${def.id}`} name="name" defaultValue={def.name} maxLength={60} required />
          {hasOptions(def.type) && (
            <>
              <label className="pm-sr-only" htmlFor={`cfo-${def.id}`}>
                Options, one per line or comma separated
              </label>
              <textarea id={`cfo-${def.id}`} name="options" defaultValue={(def.options ?? []).join("\n")} rows={3} />
            </>
          )}
          <button type="submit" className="pm-btn pm-btn-primary pm-btn-small">
            Save
          </button>
          <button type="button" className="pm-btn pm-btn-small" onClick={() => setEditing(false)}>
            Cancel
          </button>
        </form>
      </li>
    );
  }
  return (
    <li className="pm-row-between pm-card">
      <span>
        <strong>{def.name}</strong> <span className="pm-chip">{humanize(def.type)}</span>
        {hasOptions(def.type) && <span className="pm-muted"> {(def.options ?? []).join(", ")}</span>}
      </span>
      {editable && (
        <span className="pm-actions">
          <button type="button" className="pm-btn pm-btn-small" onClick={() => setEditing(true)}>
            Edit
          </button>
          <button type="button" className="pm-btn pm-btn-small" onClick={onDelete} aria-label={`Delete field ${def.name}`}>
            Delete
          </button>
        </span>
      )}
    </li>
  );
}

/** Custom field definitions. The type is fixed at creation; renaming and changing options are allowed. */
export function CustomFieldsSettings({ projectId, canManage }: { projectId: string; canManage: boolean }) {
  const res = useResource(`field-defs:${projectId}`, () => listFieldDefinitions(projectId));
  const { run, denied } = useGuardedAction(res.reload);
  const [type, setType] = useState<CustomFieldType>("text");
  const editable = canManage && !denied;
  const items = res.data ?? [];

  function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const data = new FormData(form);
    const name = String(data.get("name") ?? "").trim();
    if (name) void run(() => createFieldDefinition(projectId, name, type, parseOptions(String(data.get("options") ?? "")))).then((ok) => ok && form.reset());
  }

  return (
    <SectionFrame title="Custom fields" id="pm-cf-h" loading={res.loading} error={res.error} onRetry={res.reload}>
      {items.length === 0 && <EmptyState title="No custom fields" />}
      <ul className="pm-plain-list">
        {items.map((d) => (
          <FieldRow
            key={`${d.id}-${d.name}`}
            def={d}
            editable={editable}
            onSave={(name, options) => void run(() => updateFieldDefinition(projectId, d.id, { name, ...(hasOptions(d.type) ? { options } : {}) }))}
            onDelete={() => void run(() => deleteFieldDefinition(projectId, d.id))}
          />
        ))}
      </ul>
      {editable && (
        <form className="pm-inline-form" onSubmit={add}>
          <label className="pm-sr-only" htmlFor="cf-new-name">
            Field name
          </label>
          <input id="cf-new-name" name="name" placeholder="New field" maxLength={60} required />
          <label className="pm-sr-only" htmlFor="cf-new-type">
            Field type
          </label>
          <select id="cf-new-type" value={type} onChange={(e) => setType(e.target.value as CustomFieldType)}>
            {TYPES.map((t) => (
              <option key={t} value={t}>
                {humanize(t)}
              </option>
            ))}
          </select>
          {hasOptions(type) && (
            <>
              <label className="pm-sr-only" htmlFor="cf-new-options">
                Options
              </label>
              <input id="cf-new-options" name="options" placeholder="Options, comma separated" />
            </>
          )}
          <button type="submit" className="pm-btn">
            Add field
          </button>
        </form>
      )}
    </SectionFrame>
  );
}
