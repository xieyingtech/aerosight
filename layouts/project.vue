<script setup lang="ts">
const route = useRoute();
const { data: teams } = useFetch("/api/teams");

// 当前选中的团队
const selectedTeam = ref(route.params.namespace);
const projectId = route.params.id;

// 创建团队选择下拉菜单的选项
const teamOptions = computed(() => {
  if (!teams.value) return [];
  return teams.value.map((team) => ({
    label: team.name,
    value: team.namespace,
    command: () => navigateTo(`/teams/${team.namespace}`),
  }));
});

// 监听团队变化
watch(selectedTeam, (newValue) => {
  if (newValue) {
    navigateTo(`/teams/${newValue}`);
  }
});

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
  <NuxtLayout name="default">
    <template #header>
      <div class="flex items-center justify-between px-4 py-2 bg-gray-50 border-b">
        <div class="flex items-center gap-2">
          <Dropdown
            v-if="teamOptions.length > 0"
            :options="teamOptions"
            v-model="selectedTeam"
            optionLabel="label"
            optionValue="value"
            placeholder="选择团队"
          />
          <span class="text-gray-400 mx-2">/</span>
          <span class="font-medium">{{ project?.name }}</span>
        </div>
        <Button 
          icon="i-ri-arrow-left-line"
          text
          @click="navigateTo(`/teams/${route.params.namespace}`)"
          label="返回团队"
        />
      </div>
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
