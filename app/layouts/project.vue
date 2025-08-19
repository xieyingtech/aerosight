<script setup lang="ts">
const route = useRoute();
const projectId = route.params.id;

// 获取当前项目信息
const { data: project } = useFetch(
  `/api/teams/${route.params.namespace}/projects/${projectId}`
);

// 项目菜单项
const items = computed(() => [
  {
    label: "概述",
    icon: "i-ri-dashboard-line",
    route: `/teams/${route.params.namespace}/projects/${projectId}`,
  },
  {
    label: "布防点",
    icon: "i-ri-map-pin-line",
    route: `/teams/${route.params.namespace}/projects/${projectId}/poi`,
  },
  {
    label: "任务",
    icon: "i-ri-task-line",
    route: `/teams/${route.params.namespace}/projects/${projectId}/tasks`,
  },
  {
    label: "事件",
    icon: "i-ri-calendar-event-line",
    route: `/teams/${route.params.namespace}/projects/${projectId}/events`,
  },
  {
    label: "设置",
    icon: "i-ri-settings-line",
    route: `/teams/${route.params.namespace}/projects/${projectId}/settings`,
  },
]);
</script>

<template>
  <NuxtLayout
    name="default"
    :breadcrumb=" [
      { label: String(route.params.namespace), route: `/teams/${route.params.namespace}` },
      { label: project?.name, route: `/teams/${route.params.namespace}/projects/${projectId}` },
    ]"
    :title="project?.name"
  >
    <template #header>
      <UTabGroup :items="items" class="mb-2">
        <template #default="{ item }">
          <NuxtLink :to="item.route" class="flex items-center gap-2 px-4 py-2">
            <UIcon :name="item.icon" class="w-5 h-5" />
            <span>{{ item.label }}</span>
          </NuxtLink>
        </template>
      </UTabGroup>
    </template>
    <slot />
  </NuxtLayout>
</template>

<style scoped>
.layout-wrapper {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.content {
  flex-grow: 1;
}
</style>
