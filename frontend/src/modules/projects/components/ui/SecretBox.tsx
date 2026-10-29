"use client";

import { useState } from "react";

interface Props {
  title: string;
  secret: string;
  note?: string;
  onDismiss: () => void;
}

/**
 * Shows a secret exactly once. It lives only in the memory of the component that created it: it is never written to
 * storage and disappears when dismissed or when the page is left.
 */
export function SecretBox({ title, secret, note, onDismiss }: Props) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  return (
    <div className="pm-card pm-secret" role="alert">
      <strong>{title}</strong>
      <p className="pm-muted">{note ?? "Copy it now. It is shown only once and cannot be shown again."}</p>
      <div className="pm-inline-form">
        <label className="pm-sr-only" htmlFor="pm-secret-value">
          {title}
        </label>
        <input id="pm-secret-value" readOnly value={secret} onFocus={(e) => e.target.select()} className="pm-mono" />
        <button type="button" className="pm-btn" onClick={() => void copy()}>
          {copied ? "Copied" : "Copy"}
        </button>
        <button type="button" className="pm-btn" onClick={onDismiss}>
          I have saved it
        </button>
      </div>
    </div>
  );
}
