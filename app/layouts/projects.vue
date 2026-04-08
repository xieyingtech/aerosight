<script setup lang="ts">
import type { NavigationMenuItem } from "@nuxt/ui";

const { t } = useI18n();

const route = useRoute();
const { user } = useUserSession();

if (!user.value) {
  await navigateTo("/login");
}

const projectRouteId = computed(() => String(route.params.id || ""));

const scopeItems = computed<NavigationMenuItem[]>(() => [
  {
    type: "label",
    label: t("projects.index.sidebar.title"),
  },
  {
    label: t("projects.index.scopes.all"),
    icon: "i-lucide-layers-3",
    to: "/projects",
    active: route.path === "/projects",
  },
  {
    label: t("projects.index.scopes.joined"),
    icon: "i-lucide-users",
    to: "/projects?scope=joined",
    active: route.path === "/projects" && route.query.scope === "joined",
  },
  {
    label: t("projects.index.scopes.managed"),
    icon: "i-lucide-shield-check",
    to: "/projects?scope=managed",
    active: route.path === "/projects" && route.query.scope === "managed",
  },
]);

const { data: project } = await useFetch(
  () => `/api/projects/${projectRouteId.value}`,
  {
    default: () => null,
  },
);

const menuItems = computed<NavigationMenuItem[]>(() => {
  if (!project.value?.id) {
    return scopeItems.value;
  }

  const basePath = `/projects/${project.value.id}`;

  return [
    {
      type: "label",
      label: `${project.value?.name} · ${project.value?.teamName}`,
    },
    {
      label: "概览",
      icon: "i-lucide-layout-dashboard",
      to: basePath,
      active: route.path === basePath,
    },
    {
      label: "设备监控",
      icon: "i-lucide-radio",
      to: `${basePath}/devices`,
      active: route.path === `${basePath}/devices`,
    },
    {
      label: "智能体",
      icon: "i-lucide-bot",
      to: `${basePath}/agents`,
      active: route.path === `${basePath}/agents`,
    },
    {
      label: "任务编排",
      icon: "i-lucide-list-todo",
      to: `${basePath}/tasks`,
      active: route.path === `${basePath}/tasks`,
    },
    {
      label: "问题中心",
      icon: "i-lucide-bug",
      to: `${basePath}/issues`,
      active: route.path === `${basePath}/issues`,
    },
    {
      label: "素材库",
      icon: "i-lucide-folder-open",
      to: `${basePath}/assets`,
      active: route.path === `${basePath}/assets`,
    },
  ];
});
</script>

<template>
  <UPage>
    <template #left>
      <UPageAside>
        <UNavigationMenu :items="menuItems" orientation="vertical" />
      </UPageAside>
    </template>

    <slot />
  </UPage>
</template>
