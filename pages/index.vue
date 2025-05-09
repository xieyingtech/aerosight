<script setup lang="ts">
const appConfig = useAppConfig();
const { user, loggedIn } = useUserSession();
</script>

<template>
  <NuxtLink v-if="loggedIn">
    <Button>工作台</Button>
  </NuxtLink>
  <NuxtLink v-else to="/login">
    <Button>登录</Button>
  </NuxtLink>
</template>
