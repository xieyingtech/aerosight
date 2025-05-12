<script setup lang="ts">
const route = useRoute();

// 顶部菜单项
const menuItems = [
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
    label: "组织",
    icon: "i-ri-organization-chart",
    route: "/admin/teams",
  },
];
</script>

<template>
  <NuxtLayout name="default">
    <template #header>
      <!-- <Tabs value="/dashboard">
      <TabList>
        <Tab v-for="tab in items" :key="tab.label" :value="tab.route">
          <router-link
            v-if="tab.route"
            v-slot="{ href, navigate }"
            :to="tab.route"
            custom
          >
            <a
              v-ripple
              :href="href"
              @click="navigate"
              class="flex items-center gap-2 text-inherit"
            >
              <i :class="tab.icon" />
              <span>{{ tab.label }}</span>
            </a>
          </router-link>
        </Tab>
      </TabList>
    </Tabs> -->
    </template>
  </NuxtLayout>
</template>
