import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { Shell } from "@/modules/devtest/shell";
import "@/modules/devtest/devtest.css";

export const metadata: Metadata = {
  title: "JAAS Module Verification (temporary)",
  robots: { index: false, follow: false },
};

export default function DevTestLayout({ children }: { children: React.ReactNode }) {
  // Temporary QA console: never reachable in a production build unless explicitly enabled.
  if (process.env.NODE_ENV === "production" && process.env.NEXT_PUBLIC_ENABLE_DEV_TEST !== "1") {
    notFound();
  }
  return <Shell>{children}</Shell>;
}
