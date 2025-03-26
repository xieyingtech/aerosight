<script setup lang="ts">
const { data, error } = useFetch("/api/admin/users");

definePageMeta({ layout: "admin" });
</script>

<template>
  <ElTable :data="data ?? []">
    <ElTableColumn prop="id" label="ID" />
    <ElTableColumn prop="username" label="Username" />
    <ElTableColumn prop="email" label="Email" />
    <ElTableColumn prop="phone" label="phone" />
    <ElTableColumn prop="admin" label="Admin" />
  </ElTable>
</template>
