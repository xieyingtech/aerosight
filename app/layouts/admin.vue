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
      <Menubar :model="items" class="rounded-0 b-x-0 b-t-0">
        <template #item="{ item, props, hasSubmenu }">
          <NuxtLink
            v-if="item.route"
            v-slot="{ href, navigate }"
            :to="item.route"
            custom
          >
            <a v-ripple :href="href" v-bind="props.action" @click="navigate">
              <span :class="item.icon" class="mr-2" />
              <span>{{ item.label }}</span>
            </a>
          </NuxtLink>
          <a
            v-else-if="item.items && item.items.length"
            v-ripple
            v-bind="props.action"
          >
            <span :class="item.icon" class="mr-2" />
            <span>{{ item.label }}</span>
            <span v-if="hasSubmenu" class="i-ri-arrow-down-s-line ml-2" />
          </a>
          <a
            v-else
            v-ripple
            :href="item.url"
            :target="item.target"
            v-bind="props.action"
          >
            <span :class="item.icon" class="mr-2" />
            <span>{{ item.label }}</span>
          </a>
        </template>
      </Menubar>
    </template>
    <slot />
  </NuxtLayout>
</template>
