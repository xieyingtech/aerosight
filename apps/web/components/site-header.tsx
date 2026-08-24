import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";

export function SiteHeader() {
  return (
    <header className="flex h-16 shrink-0 items-center gap-2">
      <div className="flex items-center gap-2 px-4">
        <SidebarTrigger className="-ml-1" />
        <Separator className="mr-2 data-vertical:h-4 data-vertical:self-auto" orientation="vertical" />
        <span className="font-semibold">AeroSight</span>
      </div>
    </header>
  );
}
