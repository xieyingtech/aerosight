<script setup lang="ts">
import type { NuxtError } from "#app";

const props = defineProps({
  error: Object as () => NuxtError,
});

console.error(props.error);
</script>

<template>
  <NuxtLayout>
    <ElEmpty :description="error?.message">
      <ElButton type="primary" @click="navigateTo('/')">返回首页</ElButton>
    </ElEmpty>
  </NuxtLayout>
</template>
