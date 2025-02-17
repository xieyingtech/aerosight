<script setup lang="ts">
const { user, clear } = useUserSession();
</script>

<template>
  {{ user }}
  <ElButton @click="clear">退出登录</ElButton>
</template>
