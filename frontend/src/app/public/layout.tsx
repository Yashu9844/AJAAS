import type { Metadata } from "next";
import type { ReactNode } from "react";
import "@/modules/projects/styles/base.css";
import "@/modules/projects/styles/board.css";
import "@/modules/projects/styles/views.css";

// Public pages are never indexed and never send the page address (which holds a secret token) as a referrer.
export const metadata: Metadata = {
  title: "Shared project",
  robots: { index: false, follow: false, nocache: true },
  referrer: "no-referrer",
};

export default function PublicLayout({ children }: { children: ReactNode }) {
  return (
    <div className="pm-root">
      <main className="pm-main">{children}</main>
    </div>
  );
}
