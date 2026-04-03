<script setup lang="ts">
import type { NavigationMenuItem } from "@nuxt/ui";
const { user } = useUserSession();

if (!user.value) {
  navigateTo("/login");
}

const navItems = computed(() => {
  const items = [
    { label: "数据看板", icon: "i-lucide-layout-dashboard", to: "/console" },
    { label: "业务", type: "label" },
    { label: "设备监控", icon: "i-lucide-radio", to: "/console/devices" },
    { label: "智能体", icon: "i-lucide-bot", to: "/console/agents" },
    { label: "任务编排", icon: "i-lucide-list-todo", to: "/console/tasks" },
    { label: "问题中心", icon: "i-lucide-bug", to: "/console/issues" },
    { label: "素材库", icon: "i-lucide-folder-open", to: "/console/assets" },
  ] as NavigationMenuItem[];

  if (user.value?.role === "admin") {
    items.push(
      { label: "管理", type: "label" },
      {
        label: "团队成员",
        icon: "i-lucide-users",
        to: "/console/teams",
      },
      {
        label: "系统设置",
        icon: "i-lucide-settings",
        to: "/console/settings",
      },
    );
  }

  return items;
});
</script>

<template>
  <NuxtLayout name="default">
    <UPage>
      <template #left>
        <UPageAside>
          <UNavigationMenu :items="navItems" orientation="vertical" />
        </UPageAside>
      </template>

      <slot />
    </UPage>
  </NuxtLayout>
</template>
