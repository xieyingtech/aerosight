import { ClientAppShell } from "@/components/client-app-shell";

export default function Layout({ children }: { children: React.ReactNode }) {
  return <ClientAppShell>{children}</ClientAppShell>;
}
