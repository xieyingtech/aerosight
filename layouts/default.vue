<script setup lang="ts">
const { user } = useUserSession();

const collapsed = ref(true);
</script>

<template>
  <ElContainer class="h-100vh">
    <ElHeader>
      <ElMenu mode="horizontal">
        <ElMenuItem @click="navigateTo('/')">协盈穹目</ElMenuItem>
        <ElMenuItem>工作台</ElMenuItem>
        <ElMenuItem @click="navigateTo('/profile')">用户中心</ElMenuItem>
      </ElMenu>
    </ElHeader>
    <ElContainer>
      <ElMenu class="flex flex-col" :collapse="collapsed">
        <ElMenuItem>
          <i class="el-icon i-ri-home-fill"></i>
          <template #title>主菜单</template>
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
  </ElContainer>
</template>

<style scoped>
.el-header {
  padding: 0;
}

.el-menu--horizontal > .el-menu-item:nth-child(1) {
  margin-right: auto;
}

.el-menu--vertical > .el-menu-item:nth-last-child(1) {
  margin-top: auto;
}

.el-menu--vertical:not(.el-menu--collapse) {
  width: 200px;
}
</style>
