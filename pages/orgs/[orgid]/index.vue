<script setup lang="ts">
definePageMeta({
  layout: "orgs",
});

const route = useRoute();
const orgId = parseInt(route.params.orgid as string);

// 获取组织详情
const { data: organization, pending, error } = await useFetch(`/api/organizations/${orgId}`, {
  key: `organization-${orgId}`,
});

// 获取组织的项目统计
const projectCount = computed(() => organization.value?.projects?.length || 0);

// 获取组织的POI统计
const poiCount = computed(() => {
  if (!organization.value?.projects) return 0;
  return organization.value.projects.reduce((acc, project) => acc + (project.poi?.length || 0), 0);
});

// 获取组织的成员统计
const memberCount = computed(() => organization.value?.memberships?.length || 0);
</script>

<template>
  <div class="org-dashboard">
    <!-- 加载状态 -->
    <div v-if="pending" class="p-6">
      <ElSkeleton :rows="6" animated />
    </div>

    <!-- 错误状态 -->
    <ElResult
      v-else-if="error"
      icon="error"
      title="加载失败"
      :sub-title="error.message"
    />

    <!-- 组织概况 -->
    <div v-else-if="organization" class="space-y-6">
      <!-- 组织基本信息 -->
      <ElCard class="org-info">
        <template #header>
          <div class="flex justify-between items-center">
            <h2 class="text-xl font-bold">{{ organization.name }}</h2>
            <ElButton type="primary" size="small">编辑组织信息</ElButton>
          </div>
        </template>
        <ElDescriptions :column="1" border>
          <ElDescriptionsItem label="组织描述">
            {{ organization.description || '暂无描述' }}
          </ElDescriptionsItem>
          <ElDescriptionsItem label="创建时间">
            {{ new Date(organization.createdAt || Date.now()).toLocaleDateString() }}
          </ElDescriptionsItem>
        </ElDescriptions>
      </ElCard>

      <!-- 统计卡片 -->
      <div class="statistics grid grid-cols-1 md:grid-cols-3 gap-4">
        <ElCard shadow="hover" class="text-center">
          <div class="text-3xl font-bold text-blue-500">{{ projectCount }}</div>
          <div class="text-gray-600 mt-2">项目总数</div>
        </ElCard>
        
        <ElCard shadow="hover" class="text-center">
          <div class="text-3xl font-bold text-green-500">{{ poiCount }}</div>
          <div class="text-gray-600 mt-2">布防点总数</div>
        </ElCard>
        
        <ElCard shadow="hover" class="text-center">
          <div class="text-3xl font-bold text-purple-500">{{ memberCount }}</div>
          <div class="text-gray-600 mt-2">成员数量</div>
        </ElCard>
      </div>

      <!-- 地图组件 -->
      <ElCard>
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">组织地图概览</h3>
            <ElSwitch v-model="showMap" active-text="显示地图" inactive-text="隐藏地图" />
          </div>
        </template>
        <div v-if="showMap" class="map-container h-80">
          <Tmap />
        </div>
        <ElEmpty v-else description="地图已隐藏" />
      </ElCard>

      <!-- 最近项目 -->
      <ElCard v-if="organization.projects && organization.projects.length > 0">
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">最近项目</h3>
            <ElButton type="primary" size="small" @click="navigateTo(`/orgs/${orgId}/projects`)">
              查看全部
            </ElButton>
          </div>
        </template>
        <ElTable :data="organization.projects.slice(0, 5)" style="width: 100%">
          <ElTableColumn prop="name" label="项目名称" />
          <ElTableColumn prop="description" label="描述" show-overflow-tooltip />
          <ElTableColumn label="操作" width="150">
            <template #default="{ row }">
              <ElButton type="primary" link @click="navigateTo(`/orgs/${orgId}/projects/${row.id}`)">
                详情
              </ElButton>
              <ElButton type="primary" link>编辑</ElButton>
            </template>
          </ElTableColumn>
        </ElTable>
      </ElCard>
      <ElEmpty v-else description="暂无项目" />

      <!-- 组织成员 -->
      <ElCard v-if="organization.memberships && organization.memberships.length > 0">
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">组织成员</h3>
            <ElButton type="primary" size="small" @click="navigateTo(`/orgs/${orgId}/members`)">
              管理成员
            </ElButton>
          </div>
        </template>
        <ElTable :data="organization.memberships.slice(0, 5)" style="width: 100%">
          <ElTableColumn prop="user.username" label="用户名" />
          <ElTableColumn prop="name" label="角色" />
          <ElTableColumn label="权限" width="200">
            <template #default="{ row }">
              <ElTag 
                v-for="(role, index) in row.roles" 
                :key="index"
                size="small" 
                class="mr-1 mb-1"
              >
                {{ role }}
              </ElTag>
            </template>
          </ElTableColumn>
        </ElTable>
      </ElCard>
      <ElEmpty v-else description="暂无成员" />
    </div>

    <!-- 组织不存在 -->
    <ElResult
      v-else
      icon="info"
      title="组织不存在"
      sub-title="请检查您访问的组织是否正确"
    />
  </div>
</template>

<script lang="ts">
// 需要在setup外部定义状态
const showMap = ref(true);
</script>

<style scoped>
.org-dashboard {
  padding: 20px;
}
</style>
