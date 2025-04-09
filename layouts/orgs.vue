<script setup lang="ts">
const isSmallScreen = useMediaQuery("(max-width: 768px)");
const collapsed = ref(true);
const route = useRoute();
const selectedOrg = ref(route.params.orgid);

const { data } = useFetch("/api/memberships");
</script>

<template>
  <NuxtLayout name="default">
    <template #sidebar>
      <ElMenu
        class="flex flex-col h-full"
        :collapse="collapsed"
        router
        :default-active="route.path"
      >
        <ElMenuItem>
          <i class="el-icon i-ri-organization-chart"></i>
          <template #title>
            <ElSelect
              v-model="selectedOrg"
              @change="navigateTo(`/orgs/${selectedOrg}`)"
            >
              <ElOption
                v-for="membership in data"
                :key="membership.organizationId"
                :label="membership.organization.name"
                :value="membership.organizationId"
              />
            </ElSelect>
          </template>
        </ElMenuItem>
        <ElMenuItem :index="`/orgs/${route.params.orgid}`">
          <i class="el-icon i-ri-road-map-line"></i>
          <template #title>布防点</template>
        </ElMenuItem>
        <ElMenuItem>
          <i class="el-icon i-ri-rocket-line"></i>
          <template #title>任务</template>
        </ElMenuItem>
        <ElMenuItem>
          <i class="el-icon i-ri-plane-line"></i>
          <template #title>无人机</template>
        </ElMenuItem>
        <ElMenuItem>
          <i class="el-icon i-ri-user-line"></i>
          <template #title>成员</template>
        </ElMenuItem>
        <ElMenuItem>
          <i class="el-icon i-ri-settings-line"></i>
          <template #title>组织管理</template>
        </ElMenuItem>
        <ElMenuItem @click="collapsed = !collapsed">
          <i v-if="collapsed" class="el-icon i-ri-menu-unfold-fill"></i>
          <i v-else class="el-icon i-ri-menu-fold-fill"></i>
          <template #title>
            <span v-if="collapsed">展开</span>
            <span v-else>收起</span>
          </template>
        </ElMenuItem>
      </ElMenu>
    </template>
    <ElMain>
      <slot />
    </ElMain>
  </NuxtLayout>
</template>

<style scoped>
.el-header {
  padding: 0;
}

.el-menu--vertical > .el-menu-item:nth-last-child(1) {
  margin-top: auto;
}
</style>
