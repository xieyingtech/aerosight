import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "AeroSight",
  description: "Intelligent drone patrol management system"
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
