<script setup lang="ts">
const route = useRoute();

// 顶部菜单项
const menuItems = [
  {
    label: '仪表盘',
    icon: 'i-ri-dashboard-line',
    route: '/admin'
  },
  {
    label: '用户',
    icon: 'i-ri-user-settings-line',
    route: '/admin/users'
  },
  {
    label: '组织',
    icon: 'i-ri-organization-chart',
    route: '/admin/teams'
  }
];
</script>

<template>
  <div class="layout-wrapper">
    <div class="admin-header bg-primary text-white py-2 px-4 flex align-items-center">
      <h1 class="text-xl m-0">管理后台</h1>
    </div>
    
    <Menubar :model="menuItems">
      <template #item="{ item, props, hasSubmenu }">
        <router-link v-if="item.route" v-slot="{ href, navigate }" :to="item.route" custom>
          <a v-ripple :href="href" v-bind="props.action" @click="navigate">
            <span :class="item.icon" class="mr-2" />
            <span>{{ item.label }}</span>
          </a>
        </router-link>
        <a v-else v-ripple :href="item.url" :target="item.target" v-bind="props.action">
          <span :class="item.icon" class="mr-2" />
          <span>{{ item.label }}</span>
        </a>
      </template>
    </Menubar>
    
    <div class="admin-content p-4">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.layout-wrapper {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.admin-content {
  flex-grow: 1;
}

:deep(.p-menubar) {
  border-radius: 0;
}
</style>
