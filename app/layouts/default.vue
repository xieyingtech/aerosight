<script setup lang="ts">
const { site } = useAppConfig();
const { user } = useUserSession();
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
      <UAvatar v-if="user" :alt="user.name" />
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
