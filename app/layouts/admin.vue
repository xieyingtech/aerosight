<script setup lang="ts">
const route = useRoute();

// 顶部菜单项
const items = [
  {
    label: "仪表盘",
    icon: "i-ri-dashboard-line",
    route: "/admin",
  },
  {
    label: "用户",
    icon: "i-ri-user-settings-line",
    route: "/admin/users",
  },
  {
    label: "团队",
    icon: "i-ri-organization-chart",
    route: "/admin/teams",
  },
];
</script>

<template>
  <NuxtLayout name="default">
    <template #header>
      <UTabGroup :items="items" class="mb-2">
        <template #default="{ item }">
          <NuxtLink :to="item.route" class="flex items-center gap-2 px-4 py-2">
            <UIcon :name="item.icon" class="w-5 h-5" />
            <span>{{ item.label }}</span>
          </NuxtLink>
        </template>
      </UTabGroup>
    </template>
    <slot />
  </NuxtLayout>
</template>
