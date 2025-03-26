<script setup lang="ts">
const { user, clear } = useUserSession();

function logout() {
  clear();
  navigateTo("/login");
}
</script>

<template>
  {{ user }}
  <ElButton @click="logout">退出登录</ElButton>
</template>
