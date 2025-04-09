<script setup lang="ts">
const { user, clear } = useUserSession();

const appConfig = useAppConfig();
</script>

<template>
  <ElContainer class="h-100vh">
    <ElHeader>
      <ElMenu mode="horizontal" :ellipsis="false">
        <ElMenuItem @click="navigateTo('/')">
          {{ appConfig.site.title }}
        </ElMenuItem>
        <ElSubMenu index="profile" @click="navigateTo('/profile')">
          <template #title>用户</template>
          <ElMenuItem v-if="user?.admin" @click="navigateTo('/admin')"
            >管理后台</ElMenuItem
          >
          <ElMenuItem @click="clear">退出登录</ElMenuItem>
        </ElSubMenu>
      </ElMenu>
    </ElHeader>
    <ElContainer>
      <slot />
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
</style>
