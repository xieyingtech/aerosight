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
      <ElMenuItem index="/admin">
        <i class="el-icon i-ri-dashboard-3-line"></i>
        <template #title>仪表盘</template>
      </ElMenuItem>
      <ElMenuItem index="/admin/users">
        <i class="el-icon i-ri-user-settings-line"></i>
        <template #title>用户</template>
      </ElMenuItem>
      <ElMenuItem index="/admin/orgs">
        <i class="el-icon i-ri-community-line"></i>
        <template #title>组织</template>
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
