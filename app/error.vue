<script setup lang="ts">
import type { NuxtError } from "#app";
const props = defineProps({
  error: Object as () => NuxtError,
});
</script>

<template>
  <NuxtLayout>
    <div class="mx-auto p-8 text-center w-full max-w-md flex flex-col items-center justify-center min-h-[60vh]">
      <div
        class="rounded-full bg-red-100 dark:bg-red-900 flex items-center justify-center w-24 h-24 mb-6 shadow-lg"
      >
        <span class="text-5xl font-bold text-red-600 dark:text-red-300">
          {{ error?.statusCode || '错误' }}
        </span>
      </div>
      <h2 class="text-2xl font-semibold mb-2 text-gray-900 dark:text-gray-100">
        {{ error?.statusMessage || error?.message || '发生未知错误' }}
      </h2>
      <p class="text-gray-600 dark:text-gray-400 mb-6">
        {{ error?.message || '请稍后重试，或联系管理员。' }}
      </p>
      <UButton
        icon="i-ri-arrow-go-back-line"
        label="返回首页"
        size="lg"
        color="primary"
        @click="navigateTo('/')"
      />
    </div>
  </NuxtLayout>
</template>
