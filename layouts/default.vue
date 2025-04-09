<script setup lang="ts">
const { user, clear } = useUserSession();
const isSmallScreen = useMediaQuery("(max-width: 768px)");
const showSidebar = ref(false);

const appConfig = useAppConfig();
</script>

<template>
  <ElContainer class="h-100vh">
    <ElHeader>
      <ElMenu mode="horizontal" :ellipsis="false">
        <ElMenuItem class="md:hidden!" @click="showSidebar = !showSidebar">
          <i class="el-icon i-ri-menu-fill"></i>
        </ElMenuItem>
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
      <ElAside
        v-if="!isSmallScreen || showSidebar"
        class="max-md:absolute max-md:shadow top-60px bottom-0 z-10"
      >
        <slot name="sidebar" />
      </ElAside>
      <ElContainer class="z-0">
        <slot />
      </ElContainer>
    </ElContainer>
  </ElContainer>
</template>

<style scoped>
.el-header {
  padding: 0;
}

.el-menu--horizontal > .el-menu-item:nth-child(2) {
  margin-right: auto;
}

.el-aside {
  width: auto;
}
</style>
