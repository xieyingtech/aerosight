<script setup lang="ts">
const { user, loggedIn } = useUserSession();
</script>

<template>
  {{ user }}
</template>
