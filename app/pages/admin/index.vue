<script setup lang="ts">
const { user } = useUserSession();
const { data } = useFetch("/api/admin/stats");

definePageMeta({ layout: "admin" });
</script>

<template>
  <UGrid cols="1 md:4" gap="4">
    <UCard class="text-center">
      <template #header>用户</template>
      <div class="text-3xl font-bold">{{ data?.users || 0 }}</div>
      <UIcon name="i-ri-user-line" class="mt-2 text-2xl" />
    </UCard>
    <UCard class="text-center">
      <template #header>团队</template>
      <div class="text-3xl font-bold">{{ data?.teams || 0 }}</div>
      <UIcon name="i-ri-team-line" class="mt-2 text-2xl" />
    </UCard>
    <UCard class="text-center">
      <template #header>项目</template>
      <div class="text-3xl font-bold">{{ data?.projects || 0 }}</div>
      <UIcon name="i-ri-folder-line" class="mt-2 text-2xl" />
    </UCard>
    <UCard class="text-center">
      <template #header>访问量</template>
      <div class="text-3xl font-bold">0</div>
      <UIcon name="i-ri-bar-chart-line" class="mt-2 text-2xl" />
    </UCard>
  </UGrid>
</template>
