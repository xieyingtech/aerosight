<script setup lang="ts">
import "@unocss/reset/normalize.css";

const route = useRoute();
const { loggedIn } = useUserSession();

watchEffect(() => {
  if (!loggedIn.value && route.path !== "/login") {
    navigateTo("/login");
  }
});
</script>

<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
</template>

<style>
body {
  margin: 0;
}
</style>
