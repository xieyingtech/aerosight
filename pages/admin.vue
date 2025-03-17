<script setup lang="ts">
const { user } = useUserSession();

onMounted(() => {
  if (!user.value) {
    navigateTo("/login");
  }
});
</script>

<template>
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
</template>
