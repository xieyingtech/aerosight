<script setup lang="ts">
const { site } = useAppConfig();
const { user, clear } = useUserSession();

const userItems = computed(() => {
  if (!user.value) return [];
  return [
    { label: "个人中心", icon: "i-lucide-user", to: "/profile" },
    { label: "登出", icon: "i-lucide-log-out", onClick: () => clear() },
  ];
});
</script>

<template>
  <UHeader :title="site.title">
    <template #right>
      <UButton
        icon="i-lucide-package"
        color="neutral"
        variant="outline"
        to="/projects"
      />
      <UDropdownMenu v-if="user" :items="userItems" placement="bottom-end">
        <UAvatar :alt="user.name" />
      </UDropdownMenu>
      <UButton v-else variant="outline" to="/login">登录</UButton>
    </template>
    <template #body>
      <slot name="header-body" />
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
  </UFooter>
</template>
