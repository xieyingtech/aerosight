<script setup lang="ts">
const { site } = useAppConfig();
const { user } = useUserSession();

const navItems = computed(() => [
  {
    label: "首页",
    to: "/",
  },
  {
    label: "控制台",
    to: "/console",
  },
  {
    label: "文档",
    to: "/docs",
  },
]);
</script>

<template>
  <UHeader :title="site.title">
    <UNavigationMenu :items="navItems" variant="link" />
    <template #right>
      <UButton v-if="!user" variant="ghost" to="/login">登录</UButton>
      <span v-else class="text-sm text-toned">{{ user.name }}</span>
    </template>
    <template #body>
      <UNavigationMenu
        :items="navItems"
        variant="link"
        orientation="vertical"
      />
    </template>
  </UHeader>
  <UMain>
    <UContainer>
      <slot />
    </UContainer>
  </UMain>
  <UFooter>
    <template #left>
      <p class="text-sm text-muted">
        &copy; {{ new Date().getFullYear() }} {{ site.title }}. All rights
        reserved.
      </p>
    </template>
    <template #right>
      <UNavigationMenu :items="navItems" variant="link" />
    </template>
  </UFooter>
</template>
