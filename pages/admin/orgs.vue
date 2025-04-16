<script setup lang="ts">
const { user } = useUserSession();
const { data } = useFetch("/api/admin/orgs");

definePageMeta({ layout: "admin" });
</script>

<template>
  <ElTable :data="data ?? []">
    <ElTableColumn prop="id" label="ID" />
    <ElTableColumn prop="name" label="Name" />
  </ElTable>
</template>
