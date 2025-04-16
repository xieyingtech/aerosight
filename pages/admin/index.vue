<script setup lang="ts">
const { user } = useUserSession();
const { data } = useFetch("/api/admin/stats");

definePageMeta({ layout: "admin" });
</script>

<template>
  <ElRow>
    <ElCol :span="6">
      <ElStatistic title="用户" :value="data?.users" />
    </ElCol>
    <ElCol :span="6">
      <ElStatistic title="组织" :value="data?.orgs" />
    </ElCol>
    <ElCol :span="6">
      <ElStatistic title="项目" :value="data?.projects" />
    </ElCol>
    <ElCol :span="6">
      <ElStatistic title="访问量" :value="0" />
    </ElCol>
  </ElRow>
</template>
