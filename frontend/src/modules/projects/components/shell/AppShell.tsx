"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { signOut } from "../../api/session-store";
import { PeopleProvider } from "../../hooks/use-people";
import { useSession } from "../../hooks/use-session";
import { ToastProvider } from "../../hooks/use-toast";
import { ROUTES } from "../../lib/paths";
import { AuthGuard } from "../auth/AuthGuard";
import { HeaderSearch } from "./HeaderSearch";

function NavLink({ href, exact, children }: { href: string; exact?: boolean; children: ReactNode }) {
  const pathname = usePathname();
  const active = exact ? pathname === href : pathname.startsWith(href);
  return (
    <Link href={href} className="pm-nav-link" aria-current={active ? "page" : undefined}>
      {children}
    </Link>
  );
}

function Header() {
  const session = useSession();
  return (
    <header className="pm-header">
      <Link href={ROUTES.myWork} className="pm-brand">
        JAAS Projects
      </Link>
      <nav className="pm-nav" aria-label="Main">
        <NavLink href={ROUTES.myWork} exact>
          My work
        </NavLink>
        <NavLink href={ROUTES.projects}>Projects</NavLink>
      </nav>
      <HeaderSearch />
      <div className="pm-user">
        <span className="pm-muted pm-user-email" title={`${session.email} @ ${session.slug}`}>
          {session.email}
        </span>
        <button type="button" className="pm-btn pm-btn-small" onClick={() => void signOut()}>
          Sign out
        </button>
      </div>
    </header>
  );
}

/** Signed-in frame: guard, header with global search, toasts and the people directory. */
export function AppShell({ children }: { children: ReactNode }) {
  return (
    <AuthGuard>
      <ToastProvider>
        <PeopleProvider>
          <div className="pm-app">
            <Header />
            <main className="pm-main" id="pm-main">
              {children}
            </main>
          </div>
        </PeopleProvider>
      </ToastProvider>
    </AuthGuard>
  );
}
