"use client";

import { describeError } from "../../api/errors";
import { listCustomFieldDefinitions, listCustomFieldValues, setCustomFieldValues } from "../../api/tasks-api";
import { usePeople } from "../../hooks/use-people";
import { useResource } from "../../hooks/use-resource";
import { useToast } from "../../hooks/use-toast";
import type { Member } from "../../types/project";
import type { CustomFieldDefinition, CustomFieldValue, Task } from "../../types/task";
import { ErrorNotice, SkeletonRows } from "../ui/States";

type FieldValue = CustomFieldValue["value"];

interface EditorProps {
  field: CustomFieldValue;
  definition: CustomFieldDefinition | undefined;
  members: Member[];
  disabled: boolean;
  onChange: (value: FieldValue) => void;
}

function asText(v: FieldValue): string {
  return v === null || Array.isArray(v) ? "" : String(v);
}

function FieldEditor({ field, definition, members, disabled, onChange }: EditorProps) {
  const { nameOf } = usePeople();
  const id = `cf-${field.field_id}`;
  const options = definition?.options ?? [];
  switch (field.type) {
    case "checkbox":
      return <input id={id} type="checkbox" checked={field.value === true} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />;
    case "number":
      return <input id={id} type="number" defaultValue={asText(field.value)} disabled={disabled} onBlur={(e) => onChange(e.target.value === "" ? null : Number(e.target.value))} />;
    case "date":
      return <input id={id} type="date" defaultValue={asText(field.value)} disabled={disabled} onChange={(e) => onChange(e.target.value || null)} />;
    case "select":
      return (
        <select id={id} value={asText(field.value)} disabled={disabled} onChange={(e) => onChange(e.target.value || null)}>
          <option value="">-</option>
          {options.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      );
    case "user":
      return (
        <select id={id} value={asText(field.value)} disabled={disabled} onChange={(e) => onChange(e.target.value || null)}>
          <option value="">-</option>
          {members.map((m) => (
            <option key={m.user_id} value={m.user_id}>
              {nameOf(m.user_id)}
            </option>
          ))}
        </select>
      );
    case "multiselect": {
      const chosen = Array.isArray(field.value) ? field.value : [];
      return (
        <span className="pm-checks" role="group" aria-labelledby={`${id}-l`}>
          {options.map((o) => (
            <label key={o} className="pm-check">
              <input
                type="checkbox"
                checked={chosen.includes(o)}
                disabled={disabled}
                onChange={(e) => {
                  const next = e.target.checked ? [...chosen, o] : chosen.filter((x) => x !== o);
                  onChange(next.length ? next : null);
                }}
              />
              <span>{o}</span>
            </label>
          ))}
        </span>
      );
    }
    default:
      return (
        <input
          id={id}
          type={field.type === "url" ? "url" : "text"}
          defaultValue={asText(field.value)}
          disabled={disabled}
          placeholder={field.type === "url" ? "https://" : ""}
          onBlur={(e) => onChange(e.target.value.trim() === "" ? null : e.target.value.trim())}
        />
      );
  }
}

/** Custom fields (GET/PUT /tasks/{id}/custom-fields) rendered by field type. Each change is saved with the task version. */
export function CustomFieldsSection({ task, members, canWrite, onReload }: { task: Task; members: Member[]; canWrite: boolean; onReload: () => void }) {
  const toast = useToast();
  const res = useResource(`cfields:${task.id}:${task.version}`, async () => {
    const [values, defs] = await Promise.all([listCustomFieldValues(task.id), listCustomFieldDefinitions(task.project_id).catch(() => [])]);
    return { values, defs };
  });

  async function save(key: string, value: FieldValue) {
    try {
      await setCustomFieldValues(task.id, task.version, { [key]: value });
      onReload();
    } catch (e) {
      toast.show(describeError(e), "error");
      onReload();
    }
  }

  if (res.data && res.data.values.length === 0) return null;
  return (
    <section className="pm-section" aria-labelledby={`cf-${task.id}`}>
      <h3 id={`cf-${task.id}`}>Custom fields</h3>
      {res.loading && <SkeletonRows rows={2} height={32} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      <div className="pm-props">
        {(res.data?.values ?? []).map((f) => (
          <div key={`${f.field_id}-${task.version}`} className="pm-field">
            <label htmlFor={`cf-${f.field_id}`} id={`cf-${f.field_id}-l`}>
              {f.name}
            </label>
            <FieldEditor field={f} definition={res.data?.defs.find((d) => d.id === f.field_id)} members={members} disabled={!canWrite} onChange={(v) => void save(f.key, v)} />
          </div>
        ))}
      </div>
    </section>
  );
}
