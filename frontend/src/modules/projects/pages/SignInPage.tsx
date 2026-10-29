"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { describeError } from "../api/errors";
import { loadEndpoint } from "../api/config";
import { getSessionState, restoreSession, signIn } from "../api/session-store";
import { useSession } from "../hooks/use-session";
import { safeNextPath } from "../lib/paths";

function destination(): string {
  return safeNextPath(new URLSearchParams(window.location.search).get("next"));
}

/** Sign-in with organisation (tenant slug), e-mail and password. Server host and port are under "Server". */
export function SignInPage() {
  const router = useRouter();
  const session = useSession();
  const formRef = useRef<HTMLFormElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const ep = loadEndpoint();
    const form = formRef.current;
    if (form) {
      (form.elements.namedItem("host") as HTMLInputElement).value = ep.host;
      (form.elements.namedItem("port") as HTMLInputElement).value = ep.port;
    }
    if (getSessionState().status === "booting") void restoreSession();
  }, []);

  useEffect(() => {
    if (session.status === "signed-in") router.replace(destination());
  }, [session.status, router]);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    const field = (name: string) => String(data.get(name) ?? "");
    setBusy(true);
    setError("");
    try {
      await signIn(field("slug"), field("email"), field("password"), { host: field("host").trim(), port: field("port").trim() });
    } catch (err) {
      setError(describeError(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="pm-signin">
      <form ref={formRef} className="pm-card pm-signin-card" onSubmit={submit} aria-labelledby="pm-signin-title">
        <h1 id="pm-signin-title">Sign in</h1>
        <p className="pm-muted">Projects &amp; Tasks</p>
        <label className="pm-field">
          <span>Organisation</span>
          <input name="slug" autoComplete="organization" autoCapitalize="none" spellCheck={false} required placeholder="your-company" />
        </label>
        <label className="pm-field">
          <span>E-mail</span>
          <input name="email" type="email" autoComplete="username" required />
        </label>
        <label className="pm-field">
          <span>Password</span>
          <input name="password" type="password" autoComplete="current-password" required />
        </label>
        <details className="pm-details">
          <summary>Server</summary>
          <div className="pm-row">
            <label className="pm-field">
              <span>API host</span>
              <input name="host" spellCheck={false} />
            </label>
            <label className="pm-field pm-field-narrow">
              <span>Port</span>
              <input name="port" inputMode="numeric" spellCheck={false} />
            </label>
          </div>
        </details>
        {error && (
          <p className="pm-error" role="alert">
            {error}
          </p>
        )}
        <button type="submit" className="pm-btn pm-btn-primary" disabled={busy}>
          {busy ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </main>
  );
}
