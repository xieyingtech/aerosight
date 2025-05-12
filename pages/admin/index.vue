<script setup lang="ts">
const { user } = useUserSession();
// const { data } = useFetch("/api/admin/stats");

definePageMeta({ layout: "admin" });
</script>

<template>
  <!-- <div class="grid">
    <div class="col-12 md:col-3">
      <Card class="text-center">
        <template #title>用户</template>
        <template #content>
          <div class="text-3xl font-bold">{{ data?.users || 0 }}</div>
          <div class="i-ri-user-line mt-2 text-2xl"></div>
        </template>
      </Card>
    </div>
    <div class="col-12 md:col-3">
      <Card class="text-center">
        <template #title>团队</template>
        <template #content>
          <div class="text-3xl font-bold">{{ data?.teams || 0 }}</div>
          <div class="i-ri-team-line mt-2 text-2xl"></div>
        </template>
      </Card>
    </div>
    <div class="col-12 md:col-3">
      <Card class="text-center">
        <template #title>项目</template>
        <template #content>
          <div class="text-3xl font-bold">{{ data?.projects || 0 }}</div>
          <div class="i-ri-folder-line mt-2 text-2xl"></div>
        </template>
      </Card>
    </div>
    <div class="col-12 md:col-3">
      <Card class="text-center">
        <template #title>访问量</template>
        <template #content>
          <div class="text-3xl font-bold">0</div>
          <div class="i-ri-bar-chart-line mt-2 text-2xl"></div>
        </template>
      </Card>
    </div>
  </div> -->
</template>
