<script setup lang="ts">
const collapsed = ref(true);
</script>

<template>
  <NuxtLayout name="default">
    <ElContainer>
      <ElMenu class="flex flex-col" :collapse="collapsed">
        <ElMenuItem>
          <i class="el-icon i-ri-home-fill"></i>
          <template #title>仪表盘</template>
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
    </ElContainer>
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
