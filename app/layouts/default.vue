<script setup lang="ts">
const props = defineProps<{
  breadcrumb?: Array<{
    label: string;
    route?: string;
    icon?: string;
  }>;
  title?: string;
}>();

const { user, loggedIn, clear } = useUserSession();
const { data: profile } = loggedIn
  ? useFetch("/api/user", { watch: [user] })
  : {};
const appConfig = useAppConfig();
const route = useRoute();

const { data: teams } = useFetch("/api/teams", { immediate: loggedIn.value });

// 当前选中的团队
const selectedTeam = ref(route.params.namespace as string | undefined);

// 监听路由变化，更新 selectedTeam
watch(
  () => route.params.namespace,
  (newNamespace) => {
    selectedTeam.value = newNamespace as string | undefined;
  }
);

// 监听 selectedTeam 变化（由Dropdown引起），执行导航
watch(selectedTeam, (newNamespace, oldNamespace) => {
  if (
    newNamespace &&
    newNamespace !== oldNamespace &&
    newNamespace !== route.params.namespace
  ) {
    navigateTo(`/teams/${newNamespace}`);
  }
});

// 创建团队选择下拉菜单的选项
const teamOptionsForDropdown = computed(() => {
  if (!teams.value) return [];
  return teams.value.map((team: any) => ({
    label: team.name,
    value: team.namespace,
  }));
});

useHead({
  title: props.title
    ? `${props.title} | ${appConfig.site.title}`
    : appConfig.site.title,
});

const userMenuItems = computed(() => [
  {
    label: "个人中心",
    icon: "i-ri-user-settings-line",
    to: "/profile",
  },
  {
    label: "创建团队",
    icon: "i-ri-add-circle-line",
    to: "/teams/create",
  },
  {
    label: "退出登录",
    icon: "i-ri-logout-box-line",
    onSelect: () => {
      clear();
      navigateTo("/");
    },
  },
]);
</script>

<template>
  <div class="min-h-screen bg-gray-50 dark:bg-gray-950 text-gray-900 dark:text-gray-100">
    <header class="bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-800">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div class="flex justify-between items-center h-16">
          <!-- 左侧：面包屑导航 -->
          <div class="flex items-center space-x-4">
            <NuxtLink to="/" class="flex items-center text-gray-500 hover:text-gray-700">
              <UIcon name="i-ri-home-line" class="w-5 h-5" />
            </NuxtLink>
            
            <template v-if="breadcrumb && breadcrumb.length > 0">
              <template v-for="(item, index) in breadcrumb" :key="index">
                <UIcon name="i-ri-arrow-right-s-line" class="w-4 h-4 text-gray-400" />
                <NuxtLink 
                  v-if="item.route" 
                  :to="item.route" 
                  class="flex items-center text-sm font-medium text-gray-700 hover:text-gray-900"
                >
                  <UIcon v-if="item.icon" :name="item.icon" class="w-4 h-4 mr-1" />
                  {{ item.label }}
                </NuxtLink>
                <span v-else class="flex items-center text-sm font-medium text-gray-900">
                  <UIcon v-if="item.icon" :name="item.icon" class="w-4 h-4 mr-1" />
                  {{ item.label }}
                </span>
              </template>
            </template>
          </div>

          <!-- 右侧：用户菜单 -->
          <div class="flex items-center space-x-4">
            <template v-if="user">
              <!-- 团队选择下拉框 -->
              <USelectMenu
                v-if="loggedIn && teamOptionsForDropdown.length > 0"
                v-model="selectedTeam"
                :options="teamOptionsForDropdown"
                placeholder="选择团队"
                class="w-48"
              />
              
              <!-- 用户头像和菜单 -->
              <UDropdownMenu 
                :items="[userMenuItems]" 
                :content="{ align: 'end' }"
                :ui="{ content: 'w-48' }"
              >
                <UAvatar
                  :alt="profile?.name || profile?.username || 'User'"
                  class="cursor-pointer"
                >
                  {{ (profile?.name || profile?.username)?.charAt(0) || 'U' }}
                </UAvatar>
              </UDropdownMenu>
            </template>
            
            <UButton
              v-else
              label="登录"
              icon="i-ri-login-box-line"
              @click="navigateTo('/login')"
            />
          </div>
        </div>
      </div>
      
      <slot name="header" />
    </header>

    <main class="max-w-7xl mx-auto py-6 px-4 sm:px-6 lg:px-8 bg-white dark:bg-gray-900 text-gray-900 dark:text-gray-100">
      <slot />
    </main>
  </div>
</template>

<style scoped></style>
