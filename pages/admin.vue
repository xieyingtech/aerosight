<script setup lang="ts">
const { user } = useUserSession();

onMounted(() => {
  if (!user.value) {
    navigateTo("/login");
  }
});
</script>

<template>
  <div>
    <h1>Admin Page</h1>
    <p>Welcome, {{ user }}</p>
  </div>
</template>
