import type { Metadata } from "next";
import { Shell } from "@/modules/devtest/shell";
import "@/modules/devtest/devtest.css";

export const metadata: Metadata = {
  title: "JAAS Module Verification (temporary)",
  robots: { index: false, follow: false },
};

export default function DevTestLayout({ children }: { children: React.ReactNode }) {
  return <Shell>{children}</Shell>;
}
