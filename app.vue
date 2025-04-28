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

@media (max-width: 767.9px) {
  .el-dialog {
    width: 100% !important;
  }
}
</style>
