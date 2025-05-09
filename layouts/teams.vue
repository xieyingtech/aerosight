<script setup lang="ts">
const route = useRoute();
const selectedTeam = ref(route.params.teamid);

const { data } = useFetch("/api/memberships");

// 创建团队选择下拉菜单的选项
const teamOptions = computed(() => {
  if (!data.value) return [];
  return data.value.map(membership => ({
    label: membership.team.name,
    value: membership.teamId,
    command: () => navigateTo(`/teams/${membership.teamId}`)
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
    label: '团队选择',
    icon: 'i-ri-team-line',
    items: teamOptions.value
  },
  {
    label: '布防点',
    icon: 'i-ri-map-pin-line',
    route: `/teams/${route.params.teamid}`
  },
  {
    label: '任务',
    icon: 'i-ri-flag-line',
    route: `/teams/${route.params.teamid}/tasks`
  },
  {
    label: '无人机',
    icon: 'i-ri-plane-line',
    route: `/teams/${route.params.teamid}/drones`
  },
  {
    label: '成员',
    icon: 'i-ri-user-3-line',
    route: `/teams/${route.params.teamid}/members`
  },
  {
    label: '团队管理',
    icon: 'i-ri-settings-line',
    route: `/teams/${route.params.teamid}/settings`
  }
]);
</script>

<template>
  <div class="layout-wrapper">
    <Menubar :model="items">
      <template #item="{ item, props, hasSubmenu }">
        <router-link v-if="item.route" v-slot="{ href, navigate }" :to="item.route" custom>
          <a v-ripple :href="href" v-bind="props.action" @click="navigate">
            <span :class="item.icon" class="mr-2" />
            <span>{{ item.label }}</span>
          </a>
        </router-link>
        <a v-else-if="item.items && item.items.length" v-ripple v-bind="props.action">
          <span :class="item.icon" class="mr-2" />
          <span>{{ item.label }}</span>
          <span v-if="hasSubmenu" class="i-ri-arrow-down-s-line ml-2" />
        </a>
        <a v-else v-ripple :href="item.url" :target="item.target" v-bind="props.action">
          <span :class="item.icon" class="mr-2" />
          <span>{{ item.label }}</span>
        </a>
      </template>
    </Menubar>
    
    <div class="content p-4">
      <slot />
    </div>
  </div>
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
