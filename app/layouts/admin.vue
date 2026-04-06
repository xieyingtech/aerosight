<script setup lang="ts">
import type { NavigationMenuItem } from "@nuxt/ui";

const { user, fetch } = useUserSession();

if (!user.value) {
  await fetch();
}

if (!user.value) {
  await navigateTo("/login");
}

if (user.value?.role !== "admin") {
  await navigateTo("/projects");
}

const navItems = computed<NavigationMenuItem[]>(() => [
  {
    label: "管理总览",
    icon: "i-lucide-layout-dashboard",
    to: "/admin",
  },
  {
    label: "管理模块",
    type: "label",
  },
  {
    label: "用户管理",
    icon: "i-lucide-users",
    to: "/admin/users",
  },
  {
    label: "团队管理",
    icon: "i-lucide-building-2",
    to: "/admin/teams",
  },
  {
    label: "项目管理",
    icon: "i-lucide-folders",
    to: "/admin/projects",
  },
]);
</script>

<template>
  <NuxtLayout name="default">
    <template #header-body>
      <UNavigationMenu :items="navItems" orientation="vertical" />
    </template>
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
