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

// 需要在setup内部定义状态
const showMap = ref(true);
</script>

<template>
  <div class="team-dashboard">
    <!-- 加载状态 -->
    <div v-if="pending" class="p-6">
      <Skeleton :rows="6" animation="wave" />
    </div>

    <!-- 错误状态 -->
    <Message v-else-if="error" severity="error" :closable="false">
      <template #content>
        <div class="flex flex-column align-items-center">
          <h3>加载失败</h3>
          <p>{{ error.message }}</p>
        </div>
      </template>
    </Message>

    <!-- 团队概况 -->
    <div v-else-if="team" class="space-y-6">
      <!-- 团队基本信息 -->
      <Card class="team-info">
        <template #header>
          <div class="flex justify-between items-center">
            <h2 class="text-xl font-bold">{{ team.name }}</h2>
            <Button label="编辑团队信息" size="small" />
          </div>
        </template>
        <template #content>
          <div class="grid">
            <div class="col-12 field">
              <label class="font-bold mb-2">团队描述</label>
              <div>{{ team.description || '暂无描述' }}</div>
            </div>
            <div class="col-12 field">
              <label class="font-bold mb-2">创建时间</label>
              <div>{{ new Date(team.createdAt || Date.now()).toLocaleDateString() }}</div>
            </div>
          </div>
        </template>
      </Card>

      <!-- 统计卡片 -->
      <div class="statistics grid">
        <div class="col-12 md:col-4 p-2">
          <Card class="text-center shadow-1 h-full">
            <template #content>
              <div class="text-3xl font-bold text-blue-500">{{ projectCount }}</div>
              <div class="text-gray-600 mt-2">项目总数</div>
            </template>
          </Card>
        </div>
        
        <div class="col-12 md:col-4 p-2">
          <Card class="text-center shadow-1 h-full">
            <template #content>
              <div class="text-3xl font-bold text-green-500">{{ poiCount }}</div>
              <div class="text-gray-600 mt-2">布防点总数</div>
            </template>
          </Card>
        </div>
        
        <div class="col-12 md:col-4 p-2">
          <Card class="text-center shadow-1 h-full">
            <template #content>
              <div class="text-3xl font-bold text-purple-500">{{ memberCount }}</div>
              <div class="text-gray-600 mt-2">成员数量</div>
            </template>
          </Card>
        </div>
      </div>

      <!-- 地图组件 -->
      <Card>
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">团队地图概览</h3>
            <InputSwitch v-model="showMap" />
            <span class="ml-2">{{ showMap ? '显示地图' : '隐藏地图' }}</span>
          </div>
        </template>
        <template #content>
          <div v-if="showMap" class="map-container h-80">
            <Tmap />
          </div>
          <div v-else class="empty-container flex align-items-center justify-content-center p-5 flex-column">
            <i class="pi pi-map-marker text-4xl text-gray-400 mb-3"></i>
            <span class="text-gray-400">地图已隐藏</span>
          </div>
        </template>
      </Card>

      <!-- 最近项目 -->
      <Card v-if="team.projects && team.projects.length > 0">
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">最近项目</h3>
            <Button label="查看全部" size="small" @click="navigateTo(`/teams/${teamId}/projects`)" />
          </div>
        </template>
        <template #content>
          <DataTable :value="team.projects.slice(0, 5)" stripedRows>
            <Column field="name" header="项目名称" />
            <Column field="description" header="描述">
              <template #body="slotProps">
                <Tooltip :value="slotProps.data.description">
                  <div class="text-overflow-ellipsis">{{ slotProps.data.description }}</div>
                </Tooltip>
              </template>
            </Column>
            <Column header="操作" :style="{ width: '150px' }">
              <template #body="slotProps">
                <Button label="详情" link @click="navigateTo(`/teams/${teamId}/projects/${slotProps.data.id}`)" />
                <Button label="编辑" link />
              </template>
            </Column>
          </DataTable>
        </template>
      </Card>
      <div v-else class="empty-container flex align-items-center justify-content-center p-5 flex-column">
        <i class="pi pi-folder-open text-4xl text-gray-400 mb-3"></i>
        <span class="text-gray-400">暂无项目</span>
      </div>

      <!-- 团队成员 -->
      <Card v-if="team.memberships && team.memberships.length > 0">
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">团队成员</h3>
            <Button label="管理成员" size="small" @click="navigateTo(`/teams/${teamId}/members`)" />
          </div>
        </template>
        <template #content>
          <DataTable :value="team.memberships.slice(0, 5)" stripedRows>
            <Column field="user.username" header="用户名" />
            <Column field="name" header="角色" />
            <Column header="权限" :style="{ width: '200px' }">
              <template #body="slotProps">
                <div class="flex flex-wrap">
                  <Tag v-for="(role, index) in slotProps.data.roles" 
                    :key="index"
                    severity="info"
                    class="mr-1 mb-1"
                  >
                    {{ role }}
                  </Tag>
                </div>
              </template>
            </Column>
          </DataTable>
        </template>
      </Card>
      <div v-else class="empty-container flex align-items-center justify-content-center p-5 flex-column">
        <i class="pi pi-users text-4xl text-gray-400 mb-3"></i>
        <span class="text-gray-400">暂无成员</span>
      </div>
    </div>

    <!-- 团队不存在 -->
    <div v-else class="flex align-items-center justify-content-center p-5 flex-column">
      <i class="i-ri-information-line text-5xl text-blue-500 mb-3"></i>
      <h3>团队不存在</h3>
      <p class="text-gray-600">请检查您访问的团队是否正确</p>
    </div>
  </div>
</template>

<style scoped>
.team-dashboard {
  padding: 20px;
}
.text-overflow-ellipsis {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 300px;
}
</style>
