<script setup lang="ts">
const { data: overview, pending } = await useFetch("/api/admin/overview", {
  default: () => ({
    users: 0,
    teams: 0,
    projects: 0,
  }),
});
</script>

<template>
  <UPageHeader title="数据看板" description="当前平台核心数据统计" />
  <UPageBody>
    <UPageGrid>
      <UCard>
        <template #header>
          <div class="text-sm text-muted">用户总数</div>
        </template>
        <div class="text-3xl font-semibold">
          {{ pending ? "-" : overview.users }}
        </div>
      </UCard>

      <UCard>
        <template #header>
          <div class="text-sm text-muted">团队总数</div>
        </template>
        <div class="text-3xl font-semibold">
          {{ pending ? "-" : overview.teams }}
        </div>
      </UCard>

      <UCard>
        <template #header>
          <div class="text-sm text-muted">项目总数</div>
        </template>
        <div class="text-3xl font-semibold">
          {{ pending ? "-" : overview.projects }}
        </div>
      </UCard>
    </UPageGrid>
  </UPageBody>
</template>
