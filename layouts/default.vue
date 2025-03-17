<script setup lang="ts">
const appConfig = useAppConfig();
</script>

<template>
  <ElContainer class="h-100vh">
    <ElHeader>
      <ElMenu mode="horizontal">
        <ElMenuItem @click="navigateTo('/')">
          {{ appConfig.site.title }}
        </ElMenuItem>
        <ElMenuItem @click="navigateTo('/profile')">用户中心</ElMenuItem>
      </ElMenu>
    </ElHeader>
    <slot />
  </ElContainer>
</template>

<style scoped>
.el-header {
  padding: 0;
}

.el-menu--horizontal > .el-menu-item:nth-child(1) {
  margin-right: auto;
}
</style>
