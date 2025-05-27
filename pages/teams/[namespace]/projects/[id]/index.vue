<script setup lang="ts">
definePageMeta({
  layout: "project",
});

const route = useRoute();
const namespace = route.params.namespace as string;
const projectId = route.params.id as string;

// 获取项目详情
const { data: project, pending, error } = await useFetch(
  `/api/teams/${namespace}/projects/${projectId}`,
  { key: `project-${projectId}` }
);

// 统计数据
const poiCount = computed(() => project.value?.poi?.length || 0);
const taskCount = ref(0); // 这里将来可以替换为真实数据
const eventCount = ref(0); // 这里将来可以替换为真实数据
</script>

<template>
  <div class="p-4">
    <!-- 加载状态 -->
    <div v-if="pending" class="p-4">
      <Skeleton :rows="5" animation="wave" />
    </div>

    <!-- 错误状态 -->
    <Message v-else-if="error" severity="error" :closable="false">
      {{ error.message }}
    </Message>

    <!-- 项目概览 -->
    <div v-else-if="project" class="space-y-6">
      <!-- 项目基本信息 -->
      <Card class="project-info">
        <template #header>
          <div class="flex justify-between items-center">
            <h2 class="text-xl font-bold">{{ project.name }}</h2>
            <Button label="编辑项目" size="small" icon="i-ri-edit-line" />
          </div>
        </template>
        <template #content>
          <p class="mb-4">{{ project.description || "暂无描述" }}</p>
        </template>
      </Card>

      <!-- 统计卡片 -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card>
          <template #title>
            <h3 class="text-xl font-semibold flex flex-col items-center">
              <i class="i-ri-map-pin-fill text-4xl text-primary mb-2"></i>
              <span class="text-4xl font-bold text-primary">{{ poiCount }}</span>
            </h3>
          </template>
          <template #content>
            <p class="m-0 text-center text-xl">布防点</p>
            <div class="flex justify-center mt-3">
              <Button
                label="查看布防点"
                size="small"
                @click="navigateTo(`/teams/${namespace}/projects/${projectId}/poi`)"
              />
            </div>
          </template>
        </Card>

        <Card>
          <template #title>
            <h3 class="text-xl font-semibold flex flex-col items-center">
              <i class="i-ri-task-fill text-4xl text-primary mb-2"></i>
              <span class="text-4xl font-bold text-primary">{{ taskCount }}</span>
            </h3>
          </template>
          <template #content>
            <p class="m-0 text-center text-xl">任务</p>
            <div class="flex justify-center mt-3">
              <Button
                label="查看任务"
                size="small"
                @click="navigateTo(`/teams/${namespace}/projects/${projectId}/tasks`)"
              />
            </div>
          </template>
        </Card>

        <Card>
          <template #title>
            <h3 class="text-xl font-semibold flex flex-col items-center">
              <i class="i-ri-calendar-event-fill text-4xl text-primary mb-2"></i>
              <span class="text-4xl font-bold text-primary">{{ eventCount }}</span>
            </h3>
          </template>
          <template #content>
            <p class="m-0 text-center text-xl">事件</p>
            <div class="flex justify-center mt-3">
              <Button
                label="查看事件"
                size="small"
                @click="navigateTo(`/teams/${namespace}/projects/${projectId}/events`)"
              />
            </div>
          </template>
        </Card>
      </div>

      <!-- 地图预览 -->
      <Card>
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">地图预览</h3>
            <Button
              label="查看所有布防点"
              size="small"
              @click="navigateTo(`/teams/${namespace}/projects/${projectId}/poi`)"
            />
          </div>
        </template>
        <template #content>
          <div class="map-container h-80">
            <Tmap>
              <template v-if="project.poi && project.poi.length">
                <LMarker
                  v-for="poi in project.poi"
                  :key="poi.id"
                  :lat-lng="[poi.latitude, poi.longitude]"
                >
                  <LPopup>
                    <div>
                      <h4 class="font-bold">{{ poi.name }}</h4>
                      <p>{{ poi.description }}</p>
                    </div>
                  </LPopup>
                </LMarker>
              </template>
            </Tmap>
          </div>
        </template>
      </Card>
    </div>
  </div>
</template>
