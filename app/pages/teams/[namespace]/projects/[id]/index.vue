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
      <USkeleton class="h-4 w-full" />
      <USkeleton class="h-4 w-4/5 mt-2" />
      <USkeleton class="h-4 w-3/5 mt-2" />
      <USkeleton class="h-4 w-2/3 mt-2" />
      <USkeleton class="h-4 w-1/2 mt-2" />
    </div>

    <!-- 错误状态 -->
    <UAlert v-else-if="error" color="error" variant="solid" :close-button="false">
      <template #title>错误</template>
      <template #description>{{ error.message }}</template>
    </UAlert>

    <!-- 项目概览 -->
    <div v-else-if="project" class="space-y-6">
      <!-- 项目基本信息 -->
      <UCard class="project-info">
        <template #header>
          <div class="flex justify-between items-center">
            <h2 class="text-xl font-bold">{{ project.name }}</h2>
            <UButton label="编辑项目" size="sm" icon="i-ri-edit-line" />
          </div>
        </template>
        <div class="p-4">
          <p class="mb-4">{{ project.description || "暂无描述" }}</p>
        </div>
      </UCard>

      <!-- 统计卡片 -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <UCard>
          <template #header>
            <div class="text-center">
              <UIcon name="i-ri-map-pin-fill" class="text-4xl text-primary mb-2" />
              <div class="text-4xl font-bold text-primary">{{ poiCount }}</div>
            </div>
          </template>
          <div class="text-center">
            <p class="text-xl mb-3">布防点</p>
            <UButton
              label="查看布防点"
              size="sm"
              @click="navigateTo(`/teams/${namespace}/projects/${projectId}/poi`)"
            />
          </div>
        </UCard>

        <UCard>
          <template #header>
            <div class="text-center">
              <UIcon name="i-ri-task-fill" class="text-4xl text-primary mb-2" />
              <div class="text-4xl font-bold text-primary">{{ taskCount }}</div>
            </div>
          </template>
          <div class="text-center">
            <p class="text-xl mb-3">任务</p>
            <UButton
              label="查看任务"
              size="sm"
              @click="navigateTo(`/teams/${namespace}/projects/${projectId}/tasks`)"
            />
          </div>
        </UCard>

        <UCard>
          <template #header>
            <div class="text-center">
              <UIcon name="i-ri-calendar-event-fill" class="text-4xl text-primary mb-2" />
              <div class="text-4xl font-bold text-primary">{{ eventCount }}</div>
            </div>
          </template>
          <div class="text-center">
            <p class="text-xl mb-3">事件</p>
            <UButton
              label="查看事件"
              size="sm"
              @click="navigateTo(`/teams/${namespace}/projects/${projectId}/events`)"
            />
          </div>
        </UCard>
      </div>

      <!-- 地图预览 -->
      <UCard>
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">地图预览</h3>
            <UButton
              label="查看所有布防点"
              size="sm"
              @click="navigateTo(`/teams/${namespace}/projects/${projectId}/poi`)"
            />
          </div>
        </template>
        <div class="p-4">
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
        </div>
      </UCard>
    </div>
  </div>
</template>
