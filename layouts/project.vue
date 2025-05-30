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
      <Menubar :model="items" class="rounded-0 b-x-0 b-t-0">
        <template #item="{ item, props, hasSubmenu }">
          <NuxtLink
            v-if="item.route"
            v-slot="{ href, navigate }"
            :to="item.route"
            custom
          >
            <a v-ripple :href="href" v-bind="props.action" @click="navigate">
              <span :class="item.icon" class="mr-2" />
              <span>{{ item.label }}</span>
            </a>
          </NuxtLink>
          <a
            v-else-if="item.items && item.items.length"
            v-ripple
            v-bind="props.action"
          >
            <span :class="item.icon" class="mr-2" />
            <span>{{ item.label }}</span>
            <span v-if="hasSubmenu" class="i-ri-arrow-down-s-line ml-2" />
          </a>
          <a
            v-else
            v-ripple
            :href="item.url"
            :target="item.target"
            v-bind="props.action"
          >
            <span :class="item.icon" class="mr-2" />
            <span>{{ item.label }}</span>
          </a>
        </template>
      </Menubar>
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
