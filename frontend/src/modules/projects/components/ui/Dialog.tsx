"use client";

import { useEffect, useRef, type ReactNode } from "react";

interface DialogProps {
  title: string;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  /** "drawer" slides in from the right and is used for task details. */
  variant?: "modal" | "drawer";
}

/** A modal built on the native <dialog>: focus is trapped, Escape closes, focus returns to the opener. */
export function Dialog({ title, open, onClose, children, variant = "modal" }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      className={`pm-dialog pm-dialog-${variant}`}
      aria-label={title}
      onClose={onClose}
      onClick={(e) => {
        if (e.target === ref.current) onClose();
      }}
    >
      {open && (
        <div className="pm-dialog-body">
          <header className="pm-dialog-head">
            <h2>{title}</h2>
            <button type="button" className="pm-icon-btn" aria-label="Close" onClick={onClose}>
              ×
            </button>
          </header>
          {children}
        </div>
      )}
    </dialog>
  );
}
