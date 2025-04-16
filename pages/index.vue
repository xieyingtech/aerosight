<script setup lang="ts">
const appConfig = useAppConfig();
</script>

<template>
  <div class="w-full">
    <h1 class="text-center">{{ appConfig.site.title }}</h1>
  </div>
</template>
