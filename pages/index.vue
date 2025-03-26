<script setup lang="ts">
const { data, error } = useFetch("/api/memberships");

if (error.value) {
  navigateTo("/login");
}
</script>

<template>
  {{ data }}
</template>
