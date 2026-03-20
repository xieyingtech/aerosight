<script setup lang="ts">
const { site } = useAppConfig();

const navItems = computed(() => []);
</script>

<template>
  <UHeader :title="site.title">
    <UNavigationMenu :items="navItems" />
    <template #right>
      <UButton variant="ghost">登录</UButton>
    </template>
  </UHeader>
  <UMain>
    <UContainer>
      <slot />
    </UContainer>
  </UMain>
  <UFooter>
    <p class="text-sm text-muted">
      &copy; {{ new Date().getFullYear() }} {{ site.title }}. All rights
      reserved.
    </p>
  </UFooter>
</template>
