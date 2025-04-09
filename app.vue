<script setup lang="ts">
import "@unocss/reset/normalize.css";

const appConfig = useAppConfig();

useHead({
  title: appConfig.site.title,
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

.el-menu--vertical:not(.el-menu--collapse) {
  width: 250px;
}

/* @media screen and (max-width: 768px) {
  .el-menu--horizontal {
    width: 100%;
  }
} */
</style>
