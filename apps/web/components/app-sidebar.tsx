"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  BellRingIcon, BotIcon, BoxesIcon, Building2Icon, ChevronLeftIcon, ChevronsUpDownIcon,
  CpuIcon, FolderKanbanIcon, GaugeIcon, LayoutDashboardIcon, MapIcon, PlaneTakeoffIcon,
  PlugIcon, RadioTowerIcon, SettingsIcon, SparklesIcon, UsersIcon, WaypointsIcon
} from "lucide-react";
import { NavMain } from "@/components/nav-main";
import { NavUser } from "@/components/nav-user";
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel,
  DropdownMenuSeparator, DropdownMenuTrigger
} from "@/components/ui/dropdown-menu";
import { projectNavigationHref, visibleProjectNavigation } from "@/lib/project-navigation";
import type { ProjectPermission } from "@/lib/project-permission-policy";
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
  projects,
  ...props
}: React.ComponentProps<typeof Sidebar> & {
  user: { name: string; email: string | null; role: "user" | "admin" };
  projects: Array<{
    id: number;
    name: string;
    teamName: string;
    role: "owner" | "admin" | "member";
    permissions: ProjectPermission[];
  }>;
}) {
  const pathname = usePathname();
  const projectId = Number(pathname.match(/^\/projects\/(\d+)/)?.[1]);
  const currentProject = projects.find((project) => project.id === projectId);
  const navMain = [
    { title: "项目", url: "/projects", icon: <FolderKanbanIcon /> },
    { title: "团队", url: "/teams", icon: <Building2Icon /> }
  ];
  const navAdmin = [
    { title: "管理总览", url: "/admin", icon: <LayoutDashboardIcon /> },
    { title: "用户管理", url: "/admin/users", icon: <UsersIcon /> },
    { title: "团队管理", url: "/admin/teams", icon: <Building2Icon /> },
    { title: "项目管理", url: "/admin/projects", icon: <FolderKanbanIcon /> },
    { title: "AI Provider", url: "/admin/ai-providers", icon: <SparklesIcon /> }
  ];
  const projectIcons = {
    overview: <MapIcon />, realtime: <RadioTowerIcon />, tasks: <WaypointsIcon />,
    "flight-operations": <PlaneTakeoffIcon />, geospatial: <MapIcon />,
    devices: <CpuIcon />, connectors: <PlugIcon />, issues: <BellRingIcon />, algorithms: <BoxesIcon />,
    agents: <BotIcon />, assets: <FolderKanbanIcon />, settings: <SettingsIcon />
  };
  const projectItems = currentProject
    ? visibleProjectNavigation(currentProject.role, currentProject.permissions).map((item) => ({
        title: item.title,
        url: projectNavigationHref(currentProject.id, item.segment),
        icon: projectIcons[item.key],
        exact: item.exact
      }))
    : [];

  return (
    <Sidebar collapsible="icon" variant="inset" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            {currentProject ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <SidebarMenuButton className="data-[state=open]:bg-sidebar-accent" size="lg">
                    <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                      <GaugeIcon className="size-4" />
                    </div>
                    <div className="grid flex-1 text-left text-sm leading-tight">
                      <span className="truncate font-medium">{currentProject.name}</span>
                      <span className="truncate text-xs">{currentProject.teamName}</span>
                    </div>
                    <ChevronsUpDownIcon className="ml-auto size-4" />
                  </SidebarMenuButton>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="w-64" side="right" sideOffset={6}>
                  <DropdownMenuLabel>切换项目</DropdownMenuLabel>
                  {projects.map((project) => (
                    <DropdownMenuItem asChild key={project.id}>
                      <Link href={`/projects/${project.id}`}><MapIcon />{project.name}</Link>
                    </DropdownMenuItem>
                  ))}
                  <DropdownMenuSeparator />
                  <DropdownMenuItem asChild><Link href="/projects"><ChevronLeftIcon />返回全部项目</Link></DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
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
            )}
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        {currentProject ? (
          <NavMain items={projectItems} label="项目工作台" />
        ) : (
          <>
            <NavMain items={navMain} label="导航" />
            {user.role === "admin" && <NavMain items={navAdmin} label="系统管理" />}
          </>
        )}
      </SidebarContent>
      <SidebarFooter><NavUser user={user} /></SidebarFooter>
    </Sidebar>
  );
}
