<script setup lang="ts">
import type { NuxtError } from "#app";
const props = defineProps({
  error: Object as () => NuxtError,
});
</script>

<template>
  <NuxtLayout :title="props.error?.statusCode">
    <div class="mx-auto p-4 text-center w-full max-w-md">
      <h1>{{ error?.statusCode }}</h1>
      <p>{{ error }}</p>
    </div>
  </NuxtLayout>
</template>
