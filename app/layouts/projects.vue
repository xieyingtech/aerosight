<script setup lang="ts">
import type { NavigationMenuItem } from "@nuxt/ui";

const { t } = useI18n();
const route = useRoute();
const { user } = useUserSession();

if (!user.value) {
  await navigateTo("/login");
}

const projectRouteId = computed(() => String(route.params.id));

const { data: project } = await useFetch(
  () => `/api/projects/${projectRouteId.value}`,
  {
    default: () => null,
  },
);

const menuItems = computed<NavigationMenuItem[]>(() => {
  const basePath = `/projects/${projectRouteId.value}`;

  return [
    {
      type: "label",
      label: `${project.value?.teamName}/${project.value?.name}`,
    },
    {
      label: t("projects.layout.nav.overview"),
      icon: "i-lucide-layout-dashboard",
      to: basePath,
      active: route.path === basePath,
    },
    {
      label: t("projects.layout.nav.devices"),
      icon: "i-lucide-radio",
      to: `${basePath}/devices`,
      active: route.path === `${basePath}/devices`,
    },
    {
      label: t("projects.layout.nav.agents"),
      icon: "i-lucide-bot",
      to: `${basePath}/agents`,
      active: route.path === `${basePath}/agents`,
    },
    {
      label: t("projects.layout.nav.tasks"),
      icon: "i-lucide-list-todo",
      to: `${basePath}/tasks`,
      active: route.path === `${basePath}/tasks`,
    },
    {
      label: t("projects.layout.nav.issues"),
      icon: "i-lucide-bug",
      to: `${basePath}/issues`,
      active: route.path === `${basePath}/issues`,
    },
    {
      label: t("projects.layout.nav.assets"),
      icon: "i-lucide-folder-open",
      to: `${basePath}/assets`,
      active: route.path === `${basePath}/assets`,
    },
  ];
});
</script>

<template>
  <NuxtLayout name="default">
    <template #header-body>
      <UNavigationMenu :items="menuItems" orientation="vertical" />
    </template>
    <UPage>
      <template #left>
        <UPageAside>
          <UNavigationMenu :items="menuItems" orientation="vertical" />
        </UPageAside>
      </template>

      <slot />
    </UPage>
  </NuxtLayout>
</template>
