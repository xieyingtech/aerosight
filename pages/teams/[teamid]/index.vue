<script setup lang="ts">
definePageMeta({
  layout: "teams",
});

const route = useRoute();
const teamId = parseInt(route.params.teamid as string);

// 获取团队详情
const { data: team, pending, error } = await useFetch(`/api/teams/${teamId}`, {
  key: `team-${teamId}`,
});

// 获取团队的项目统计
const projectCount = computed(() => team.value?.projects?.length || 0);

// 获取团队的POI统计
const poiCount = computed(() => {
  if (!team.value?.projects) return 0;
  return team.value.projects.reduce((acc, project) => acc + (project.poi?.length || 0), 0);
});

// 获取团队的成员统计
const memberCount = computed(() => team.value?.memberships?.length || 0);
</script>

<template>
  <div class="team-dashboard">
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

    <!-- 团队概况 -->
    <div v-else-if="team" class="space-y-6">
      <!-- 团队基本信息 -->
      <ElCard class="team-info">
        <template #header>
          <div class="flex justify-between items-center">
            <h2 class="text-xl font-bold">{{ team.name }}</h2>
            <ElButton type="primary" size="small">编辑团队信息</ElButton>
          </div>
        </template>
        <ElDescriptions :column="1" border>
          <ElDescriptionsItem label="团队描述">
            {{ team.description || '暂无描述' }}
          </ElDescriptionsItem>
          <ElDescriptionsItem label="创建时间">
            {{ new Date(team.createdAt || Date.now()).toLocaleDateString() }}
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
            <h3 class="font-bold">团队地图概览</h3>
            <ElSwitch v-model="showMap" active-text="显示地图" inactive-text="隐藏地图" />
          </div>
        </template>
        <div v-if="showMap" class="map-container h-80">
          <Tmap />
        </div>
        <ElEmpty v-else description="地图已隐藏" />
      </ElCard>

      <!-- 最近项目 -->
      <ElCard v-if="team.projects && team.projects.length > 0">
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">最近项目</h3>
            <ElButton type="primary" size="small" @click="navigateTo(`/teams/${teamId}/projects`)">
              查看全部
            </ElButton>
          </div>
        </template>
        <ElTable :data="team.projects.slice(0, 5)" style="width: 100%">
          <ElTableColumn prop="name" label="项目名称" />
          <ElTableColumn prop="description" label="描述" show-overflow-tooltip />
          <ElTableColumn label="操作" width="150">
            <template #default="{ row }">
              <ElButton type="primary" link @click="navigateTo(`/teams/${teamId}/projects/${row.id}`)">
                详情
              </ElButton>
              <ElButton type="primary" link>编辑</ElButton>
            </template>
          </ElTableColumn>
        </ElTable>
      </ElCard>
      <ElEmpty v-else description="暂无项目" />

      <!-- 团队成员 -->
      <ElCard v-if="team.memberships && team.memberships.length > 0">
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">团队成员</h3>
            <ElButton type="primary" size="small" @click="navigateTo(`/teams/${teamId}/members`)">
              管理成员
            </ElButton>
          </div>
        </template>
        <ElTable :data="team.memberships.slice(0, 5)" style="width: 100%">
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

    <!-- 团队不存在 -->
    <ElResult
      v-else
      icon="info"
      title="团队不存在"
      sub-title="请检查您访问的团队是否正确"
    />
  </div>
</template>

<script lang="ts">
// 需要在setup外部定义状态
const showMap = ref(true);
</script>

<style scoped>
.team-dashboard {
  padding: 20px;
}
</style>
