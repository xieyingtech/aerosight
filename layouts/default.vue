<script setup lang="ts">
import type { BreadcrumbProps } from "primevue";
import type { MenuItem } from "primevue/menuitem";
import Dropdown from "primevue/dropdown";

const props = defineProps<{
  breadcrumb?: BreadcrumbProps["model"];
  title?: string;
}>();

const { user, loggedIn, clear } = useUserSession();
const { data: profile } = loggedIn
  ? useFetch("/api/user", { watch: [user] })
  : {};
const appConfig = useAppConfig();
const userMenu = useTemplateRef("userMenu");
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
  return teams.value.map((team) => ({
    label: team.name,
    value: team.namespace,
  }));
});

useHead({
  title: props.title
    ? `${props.title} | ${appConfig.site.title}`
    : appConfig.site.title,
});

const userMenuItems = computed<MenuItem[]>(() => [
  {
    label: "个人中心",
    icon: "i-ri-user-settings-line",
    route: "/profile",
  },
  {
    label: "创建团队",
    icon: "i-ri-add-circle-line",
    route: "/teams/create",
    visible: loggedIn.value,
  },
  {
    label: "退出登录",
    icon: "i-ri-logout-box-line",
    command: () => {
      clear();
      navigateTo("/");
    },
    visible: loggedIn.value,
  },
]);
</script>

<template>
  <header>
    <Menubar class="rounded-0 b-x-0 not-last:b-b-0">
      <template #start>
        <div class="flex items-center gap-3">
          <Breadcrumb
            :home="{ icon: 'i-ri-home-line', route: '/' }"
            :model="breadcrumb"
          >
            <template #item="{ item, props }">
              <NuxtLink v-slot="{ href, navigate }" :to="item.route" custom>
                <a :href="href" v-bind="props.action" @click="navigate">
                  <span :class="[item.icon, 'text-color']" />
                  <span class="text-primary font-semibold">
                    {{ item.label }}
                  </span>
                </a>
              </NuxtLink>
            </template>
          </Breadcrumb>
        </div>
      </template>

      <template #end>
        <Avatar
          v-if="user"
          @click="userMenu?.toggle"
          class="cursor-pointer select-none"
        >
          {{ (profile?.name || profile?.username)?.charAt(0) }}
        </Avatar>
        <Button
          v-else
          label="登录"
          icon="i-ri-login-box-line"
          @click="navigateTo('/login')"
        />
        <Menu :model="userMenuItems" :popup="true" ref="userMenu">
          <template #start v-if="loggedIn">
            <div class="flex items-center w-full" @click.stop>
              <span class="i-ri-group-line mr-2 shrink-0"></span>
              <Dropdown
                v-model="selectedTeam"
                :options="teamOptionsForDropdown"
                optionLabel="label"
                optionValue="value"
                placeholder="选择团队"
              />
            </div>
          </template>
          <template #item="{ item, props }">
            <NuxtLink
              v-if="item.route"
              v-slot="{ href, navigate }"
              :to="item.route"
              custom
            >
              <a v-ripple :href="href" v-bind="props.action" @click="navigate">
                <span :class="item.icon" />
                <span class="ml-2">{{ item.label }}</span>
              </a>
            </NuxtLink>
            <a
              v-else
              v-ripple
              v-bind="props.action"
              :href="item.url"
              :target="item.target"
            >
              <!-- Handles command, url, target -->
              <span :class="item.icon" />
              <span class="ml-2">{{ item.label }}</span>
            </a>
          </template>
        </Menu>
      </template>
    </Menubar>
    <slot name="header" />
  </header>

  <div class="p-4">
    <slot />
  </div>
</template>

<style scoped></style>
