<script setup lang="ts">
import type { BreadcrumbProps } from "primevue";

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

useHead({
  title: props.title
    ? `${props.title} | ${appConfig.site.title}`
    : appConfig.site.title,
});

const userMenuItems = [
  {
    label: "个人中心",
    icon: "i-ri-user-settings-line",
    route: "/profile",
  },
  {
    label: "退出登录",
    icon: "i-ri-logout-box-line",
    command: () => {
      clear();
      navigateTo("/");
    },
  },
];
</script>

<template>
  <header>
    <Menubar class="rounded-0 b-x-0 not-last:b-b-0">
      <template #start>
        <Breadcrumb
          :home="{ icon: 'i-ri-home-line', route: '/' }"
          :model="breadcrumb"
        >
          <template #item="{ item, props }">
            <NuxtLink v-slot="{ href, navigate }" :to="item.route" custom>
              <a :href="href" v-bind="props.action" @click="navigate">
                <span :class="[item.icon, 'text-color']" />
                <span class="text-primary font-semibold">{{ item.label }}</span>
              </a>
            </NuxtLink>
          </template>
        </Breadcrumb>
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
          <template #item="{ item, props }">
            <NuxtLink v-slot="{ href, navigate }" :to="item.route" custom>
              <a v-ripple :href="href" v-bind="props.action" @click="navigate">
                <span :class="item.icon" />
                <span class="ml-2">{{ item.label }}</span>
              </a>
            </NuxtLink>
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
