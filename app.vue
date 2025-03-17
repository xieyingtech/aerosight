<script setup lang="ts">
import "@unocss/reset/normalize.css";
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
  width: 200px;
}
</style>
