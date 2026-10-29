"use client";

import { useEffect, useRef, type KeyboardEvent, type ReactNode } from "react";

const FOCUSABLE = 'a[href], button, input:not([type="hidden"]), select, textarea, summary, [tabindex]:not([tabindex="-1"])';

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

  /** Keeps Tab and Shift+Tab inside the dialog: the browser would otherwise move focus out to its own chrome. */
  function trapTab(e: KeyboardEvent<HTMLDialogElement>) {
    if (e.key !== "Tab" || !ref.current) return;
    const focusable = [...ref.current.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((el) => !el.hasAttribute("disabled") && el.offsetParent !== null);
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const active = document.activeElement;
    if (e.shiftKey && (active === first || active === ref.current)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && active === last) {
      e.preventDefault();
      first.focus();
    }
  }

  return (
    <dialog
      ref={ref}
      className={`pm-dialog pm-dialog-${variant}`}
      role="dialog"
      aria-modal="true"
      aria-label={title}
      onClose={onClose}
      onKeyDown={trapTab}
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
