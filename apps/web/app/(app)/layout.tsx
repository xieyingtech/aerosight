import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { requireUser } from "@/lib/data";

export default async function Layout({ children }: { children: React.ReactNode }) {
  const user = await requireUser();
  return (
    <SidebarProvider>
      <AppSidebar user={user} />
      <SidebarInset>
        <SiteHeader />
        <main className="flex flex-1 flex-col gap-4 p-4 pt-0">{children}</main>
      </SidebarInset>
    </SidebarProvider>
  );
}
