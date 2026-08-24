import type { Metadata } from "next";
import "./globals.css";
import { Geist } from "next/font/google";
import { cn } from "@/lib/utils";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";

const geist = Geist({ subsets: ["latin"], variable: "--font-sans" });

export const metadata: Metadata = {
  title: "AeroSight",
  description: "空地一体化智能感知平台"
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN" className={cn("font-sans", geist.variable)}>
      <body>
        <TooltipProvider>
          {children}
          <Toaster richColors />
        </TooltipProvider>
      </body>
    </html>
  );
}
