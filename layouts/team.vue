<script setup lang="ts">
const route = useRoute();
const { data: teams } = useFetch("/api/teams");

// 当前选中的团队
const selectedTeam = ref(route.params.namespace);

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

// 侧边菜单项
const items = computed(() => [
  {
    label: "概况",
    icon: "i-ri-dashboard-line",
    route: `/teams/${route.params.namespace}`,
  },
  {
    label: "任务",
    icon: "i-ri-task-line",
    route: `/teams/${route.params.namespace}/tasks`,
  },
  {
    label: "事件",
    icon: "i-ri-calendar-event-line",
    route: `/teams/${route.params.namespace}/events`,
  },
  {
    label: "设备",
    icon: "i-ri-device-line",
    route: `/teams/${route.params.namespace}/devices`,
  },
  {
    label: "成员",
    icon: "i-ri-user-3-line",
    route: `/teams/${route.params.namespace}/members`,
  },
  {
    label: "设置",
    icon: "i-ri-settings-line",
    route: `/teams/${route.params.namespace}/settings`,
  },
]);
</script>

<template>
  <NuxtLayout name="default">
    <template #header>
      <Menubar :model="items" class="rounded-0 b-x-0 b-t-0">
        <template #start>
          <Dropdown
            v-if="teamOptions.length > 0"
            :options="teamOptions"
            v-model="selectedTeam"
            optionLabel="label"
            optionValue="value"
            placeholder="选择团队"
          />
        </template>
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
