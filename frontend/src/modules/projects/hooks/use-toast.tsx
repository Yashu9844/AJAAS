"use client";

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";

type ToastKind = "info" | "error" | "warning";

interface ToastItem {
  id: number;
  kind: ToastKind;
  text: string;
}

interface ToastApi {
  show: (text: string, kind?: ToastKind) => void;
}

const ToastContext = createContext<ToastApi>({ show: () => undefined });
const LIFETIME_MS = 6000;
let nextId = 1;

/** A polite live region that shows short messages for a few seconds. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const dismiss = useCallback((id: number) => setItems((cur) => cur.filter((t) => t.id !== id)), []);
  const show = useCallback(
    (text: string, kind: ToastKind = "info") => {
      const id = nextId++;
      setItems((cur) => [...cur.slice(-3), { id, kind, text }]);
      setTimeout(() => dismiss(id), LIFETIME_MS);
    },
    [dismiss],
  );
  const api = useMemo(() => ({ show }), [show]);
  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className="pm-toasts" role="status" aria-live="polite">
        {items.map((t) => (
          <div key={t.id} className={`pm-toast pm-toast-${t.kind}`}>
            <span>{t.text}</span>
            <button type="button" className="pm-icon-btn" aria-label="Dismiss message" onClick={() => dismiss(t.id)}>
              ×
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  return useContext(ToastContext);
}
