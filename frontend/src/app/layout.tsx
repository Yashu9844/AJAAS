import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "JAAS Platform",
  description: "Enterprise SaaS Platform bootstrapped foundation",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}

