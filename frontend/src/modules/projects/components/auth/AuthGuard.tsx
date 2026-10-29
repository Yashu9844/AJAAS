"use client";

import { usePathname, useRouter } from "next/navigation";
import { useEffect, type ReactNode } from "react";
import { restoreSession } from "../../api/session-store";
import { useSession } from "../../hooks/use-session";
import { ROUTES, safeNextPath } from "../../lib/paths";
import { SkeletonRows } from "../ui/States";

/**
 * Renders children only for a signed-in person. On first load it restores the session from the tab's refresh token;
 * a lost or missing session redirects to the sign-in page and remembers where the person was. This is a convenience:
 * every API call is authorised by the server regardless.
 */
export function AuthGuard({ children }: { children: ReactNode }) {
  const session = useSession();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (session.status === "booting") void restoreSession();
  }, [session.status]);

  useEffect(() => {
    if (session.status !== "signed-out") return;
    const next = safeNextPath(pathname);
    router.replace(next === ROUTES.myWork ? ROUTES.signIn : `${ROUTES.signIn}?next=${encodeURIComponent(next)}`);
  }, [session.status, pathname, router]);

  if (session.status !== "signed-in") {
    return (
      <div className="pm-boot">
        <SkeletonRows rows={4} height={48} />
      </div>
    );
  }
  return <>{children}</>;
}
