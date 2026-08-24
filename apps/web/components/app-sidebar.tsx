import Link from "next/link";
import { Building2Icon, FolderKanbanIcon, LayoutDashboardIcon, PlaneTakeoffIcon, UsersIcon } from "lucide-react";
import { NavMain } from "@/components/nav-main";
import { NavUser } from "@/components/nav-user";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem
} from "@/components/ui/sidebar";

export function AppSidebar({
  user,
  ...props
}: React.ComponentProps<typeof Sidebar> & {
  user: { name: string; email: string | null; role: "user" | "admin" };
}) {
  const navMain = [
    { title: "项目", url: "/projects", icon: <FolderKanbanIcon /> },
    { title: "团队", url: "/teams", icon: <Building2Icon /> }
  ];
  const navAdmin = [
    { title: "管理总览", url: "/admin", icon: <LayoutDashboardIcon /> },
    { title: "用户管理", url: "/admin/users", icon: <UsersIcon /> },
    { title: "团队管理", url: "/admin/teams", icon: <Building2Icon /> },
    { title: "项目管理", url: "/admin/projects", icon: <FolderKanbanIcon /> }
  ];

  return (
    <Sidebar collapsible="icon" variant="inset" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild size="lg">
              <Link href="/projects">
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                  <PlaneTakeoffIcon className="size-4" />
                </div>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-medium">AeroSight</span>
                  <span className="truncate text-xs">空地一体化智能感知平台</span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavMain items={navMain} label="导航" />
        {user.role === "admin" && <NavMain items={navAdmin} label="系统管理" />}
      </SidebarContent>
      <SidebarFooter><NavUser user={user} /></SidebarFooter>
    </Sidebar>
  );
}
