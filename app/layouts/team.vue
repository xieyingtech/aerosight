<script setup lang="ts">
import USidebar from '@nuxt/ui';
import UVerticalNav from '@nuxt/ui';
const route = useRoute();

// 侧边菜单项
const items = [
  {
  type: 'link',
    label: "概况",
    icon: "i-ri-dashboard-line",
    to: `/teams/${route.params.namespace}`,
  },
  {
    type: 'item',
    label: "任务",
    icon: "i-ri-task-line",
    to: `/teams/${route.params.namespace}/tasks`,
  },
  {
    type: 'item',
    label: "事件",
    icon: "i-ri-calendar-event-line",
    to: `/teams/${route.params.namespace}/events`,
  },
  {
    type: 'item',
    label: "设备",
    icon: "i-ri-device-line",
    to: `/teams/${route.params.namespace}/devices`,
  },
  {
    type: 'item',
    label: "成员",
    icon: "i-ri-user-3-line",
    to: `/teams/${route.params.namespace}/members`,
  },
  {
    type: 'item',
    label: "设置",
    icon: "i-ri-settings-line",
    to: `/teams/${route.params.namespace}/settings`,
  },
];
</script>

<template>
  <NuxtLayout
    name="default"
    :breadcrumb="[{ label: String(route.params.namespace), route: `/teams/${route.params.namespace}` }]"
    :title="route.params.namespace"
  >
    <template #header>
  <!-- <Menubar :model="items" class="rounded-0 b-x-0 b-t-0"> -->
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
        <UTabGroup orientation="vertical" :items="items" class="w-64 min-h-screen bg-white dark:bg-gray-900 border-r border-gray-200 dark:border-gray-800">
          <template #default="{ item }">
            <NuxtLink :to="item.to" :target="item.target" class="flex items-center px-4 py-2 gap-2 hover:bg-gray-100 dark:hover:bg-gray-800 rounded transition">
              <UIcon :name="item.icon" class="text-xl" />
              <span>{{ item.label }}</span>
              <UIcon v-if="item.items && item.items.length" name="i-ri-arrow-down-s-line" class="ml-auto" />
            </NuxtLink>
          </template>
        </UTabGroup>
    </template>
    <slot />
  </NuxtLayout>
</template>

<style scoped>
.layout-wrapper {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.content {
  flex-grow: 1;
}
</style>
