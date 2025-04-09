<script setup lang="ts">
const collapsed = ref(true);
const route = useRoute();
</script>

<template>
  <NuxtLayout name="default">
    <ElMenu
      class="flex flex-col"
      :collapse="collapsed"
      router
      :default-active="route.path"
    >
      <ElMenuItem :index="`/${route.params.orgid}`">
        <i class="el-icon i-ri-home-fill"></i>
        <template #title>地图</template>
      </ElMenuItem>
      <ElMenuItem @click="collapsed = !collapsed">
        <i v-if="collapsed" class="el-icon i-ri-menu-unfold-fill"></i>
        <i v-else class="el-icon i-ri-menu-fold-fill"></i>
        <template #title>
          <span v-if="collapsed">展开</span>
          <span v-else>收起</span>
        </template>
      </ElMenuItem>
    </ElMenu>
    <ElMain>
      <slot />
    </ElMain>
  </NuxtLayout>
</template>

<style scoped>
.el-header {
  padding: 0;
}

.el-menu--vertical > .el-menu-item:nth-last-child(1) {
  margin-top: auto;
}
</style>
