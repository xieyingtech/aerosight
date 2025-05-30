<script setup lang="ts">
definePageMeta({
  layout: "project",
});

const route = useRoute();
const namespace = route.params.namespace as string;
const projectId = route.params.id as string;

// 获取项目布防点列表
const {
  data: poiList,
  pending,
  error,
  refresh,
} = await useFetch(`/api/teams/${namespace}/projects/${projectId}/poi`, {
  key: `poi-list-${projectId}`,
});

// 控制新增/编辑对话框
const showDialog = ref(false);
const isEdit = ref(false);
const currentPoi = ref(null);

// 临时标记位置（用于显示用户在地图上选中的点）
const tempMarkerPosition = ref(null);

// 打开新增对话框
const openAddDialog = () => {
  isEdit.value = false;
  currentPoi.value = {
    name: "",
    description: "",
    latitude: 0,
    longitude: 0,
  };
  tempMarkerPosition.value = null; // 重置临时标记
  showDialog.value = true;
};

// 打开编辑对话框
const openEditDialog = (poi) => {
  isEdit.value = true;
  currentPoi.value = { ...poi };
  tempMarkerPosition.value = null; // 编辑时不显示临时标记
  showDialog.value = true;
};

// 保存布防点（新增或编辑）
const savePoi = async () => {
  try {
    if (isEdit.value) {
      // 编辑现有布防点
      await $fetch(
        `/api/teams/${namespace}/projects/${projectId}/poi/${currentPoi.value.id}`,
        {
          method: "PUT",
          body: currentPoi.value,
        }
      );
    } else {
      // 添加新布防点
      await $fetch(`/api/teams/${namespace}/projects/${projectId}/poi`, {
        method: "POST",
        body: currentPoi.value,
      });
    }
    // 刷新列表
    refresh();
    showDialog.value = false;
    tempMarkerPosition.value = null; // 清除临时标记
  } catch (e) {
    // 处理错误
    console.error("保存布防点时出错:", e);
  }
};

// 删除布防点
const deletePoi = async (poiId) => {
  try {
    await $fetch(`/api/teams/${namespace}/projects/${projectId}/poi/${poiId}`, {
      method: "DELETE",
    });
    // 刷新列表
    refresh();
  } catch (e) {
    console.error("删除布防点时出错:", e);
  }
};

// 地图相关逻辑
const mapCenter = ref([39.9042, 116.4074]); // 默认北京坐标

// 如果有布防点，则更新地图中心为第一个布防点的位置
watch(poiList, (newList) => {
  if (newList && newList.length > 0) {
    mapCenter.value = [newList[0].latitude, newList[0].longitude];
  }
});

// 地图点击事件，用于添加新布防点时获取坐标
const handleMapClick = (event) => {
  if (showDialog.value && !isEdit.value) {
    // 仅在添加新布防点时响应
    currentPoi.value.latitude = event.latlng.lat;
    currentPoi.value.longitude = event.latlng.lng;
    // 设置临时标记位置
    tempMarkerPosition.value = [event.latlng.lat, event.latlng.lng];
  }
};

// 监听对话框关闭，清除临时标记
watch(showDialog, (newValue) => {
  if (!newValue) {
    tempMarkerPosition.value = null;
  }
});

// 监听坐标输入变化，更新临时标记
watch(
  [() => currentPoi.value?.latitude, () => currentPoi.value?.longitude],
  ([lat, lng]) => {
    if (
      showDialog.value &&
      !isEdit.value &&
      lat &&
      lng &&
      lat !== 0 &&
      lng !== 0
    ) {
      tempMarkerPosition.value = [lat, lng];
    }
  },
  { deep: true }
);

// 对话框内地图的中心点
const dialogMapCenter = computed(() => {
  if (currentPoi.value?.latitude && currentPoi.value?.longitude && 
      currentPoi.value.latitude !== 0 && currentPoi.value.longitude !== 0) {
    return [currentPoi.value.latitude, currentPoi.value.longitude];
  }
  // 如果有现有布防点，使用第一个布防点的位置
  if (poiList.value && poiList.value.length > 0) {
    return [poiList.value[0].latitude, poiList.value[0].longitude];
  }
  // 默认北京坐标
  return [39.9042, 116.4074];
});

// 对话框内地图点击事件
const handleDialogMapClick = (event) => {
  if (!isEdit.value) {
    console.log('地图点击事件:', event.latlng); // 添加调试信息
    currentPoi.value.latitude = event.latlng.lat;
    currentPoi.value.longitude = event.latlng.lng;
    tempMarkerPosition.value = [event.latlng.lat, event.latlng.lng];
    console.log('设置临时标记位置:', tempMarkerPosition.value); // 添加调试信息
  }
};

// 添加调试监听
watch(tempMarkerPosition, (newPos) => {
  console.log('临时标记位置变化:', newPos);
});
</script>

<template>
  <div class="p-4">
    <div class="flex justify-between items-center mb-4">
      <h1 class="text-2xl font-bold">布防点管理</h1>
      <Button label="添加布防点" icon="i-ri-add-line" @click="openAddDialog" />
    </div>

    <!-- 加载状态 -->
    <div v-if="pending" class="p-4">
      <Skeleton :rows="5" animation="wave" />
    </div>

    <!-- 错误状态 -->
    <Message v-else-if="error" severity="error" :closable="false">
      {{ error.message }}
    </Message>

    <!-- 布防点内容 -->
    <div v-else class="grid grid-cols-1 lg:grid-cols-3 gap-4">
      <!-- 布防点列表 -->
      <div class="lg:col-span-1">
        <Card>
          <template #header>
            <div class="flex justify-between items-center">
              <h3 class="font-bold">布防点列表</h3>
              <span class="text-sm text-gray-500">
                共 {{ poiList?.length || 0 }} 个布防点
              </span>
            </div>
          </template>
          <template #content>
            <div
              v-if="!poiList || poiList.length === 0"
              class="text-center py-4"
            >
              <p class="text-gray-500">
                暂无布防点，请点击"添加布防点"按钮创建
              </p>
            </div>
            <div v-else class="space-y-2 max-h-[600px] overflow-y-auto">
              <div
                v-for="poi in poiList"
                :key="poi.id"
                class="p-3 border rounded hover:bg-gray-50 cursor-pointer"
                @click="openEditDialog(poi)"
              >
                <div class="flex justify-between">
                  <h4 class="font-bold">{{ poi.name }}</h4>
                  <div class="flex gap-1">
                    <Button
                      icon="i-ri-edit-line"
                      text
                      rounded
                      aria-label="编辑"
                      @click.stop="openEditDialog(poi)"
                    />
                    <Button
                      icon="i-ri-delete-bin-line"
                      text
                      rounded
                      severity="danger"
                      aria-label="删除"
                      @click.stop="deletePoi(poi.id)"
                    />
                  </div>
                </div>
                <p class="text-sm text-gray-600 mt-1">
                  {{ poi.description || "无描述" }}
                </p>
                <div class="text-xs text-gray-500 mt-2">
                  <span>经度: {{ poi.longitude.toFixed(6) }}</span>
                  <span class="ml-2">纬度: {{ poi.latitude.toFixed(6) }}</span>
                </div>
              </div>
            </div>
          </template>
        </Card>
      </div>

      <!-- 地图展示 -->
      <div class="lg:col-span-2">
        <Card>
          <template #header>
            <div class="flex justify-between items-center">
              <h3 class="font-bold">地图视图</h3>
              <div class="text-sm text-gray-500">
                <i class="i-ri-information-line mr-1"></i>
                <span>点击布防点标记查看详情和操作</span>
              </div>
            </div>
          </template>
          <template #content>
            <div class="h-[600px]">
              <Tmap :center="mapCenter" :zoom="12">
                <LMarker
                  v-for="poi in poiList"
                  :key="poi.id"
                  :lat-lng="[poi.latitude, poi.longitude]"
                >
                  <LPopup>
                    <div>
                      <p class="font-bold mb-1">{{ poi.name }}</p>
                      <p class="text-sm text-gray-600 mb-2">{{ poi.description || "无描述" }}</p>
                      <div class="text-xs text-gray-500 mb-3">
                        <p>纬度: {{ poi.latitude.toFixed(6) }}</p>
                        <p>经度: {{ poi.longitude.toFixed(6) }}</p>
                      </div>
                      <div class="flex gap-2">
                        <Button
                          label="编辑"
                          icon="i-ri-edit-line"
                          size="small"
                          @click="openEditDialog(poi)"
                        />
                        <Button
                          label="删除"
                          icon="i-ri-delete-bin-line"
                          size="small"
                          severity="danger"
                          @click="deletePoi(poi.id)"
                        />
                      </div>
                    </div>
                  </LPopup>
                </LMarker>
              </Tmap>
            </div>
          </template>
        </Card>
      </div>
    </div>

    <!-- 添加/编辑布防点对话框 -->
    <Dialog
      v-model:visible="showDialog"
      :header="isEdit ? '编辑布防点' : '添加布防点'"
      :modal="true"
      :style="{ width: '800px' }"
    >
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mt-2">
        <!-- 左侧：表单 -->
        <div class="space-y-4">
          <div class="field">
            <label for="name" class="block font-medium mb-1">名称</label>
            <InputText id="name" v-model="currentPoi.name" class="w-full" />
          </div>
          <div class="field">
            <label for="description" class="block font-medium mb-1">描述</label>
            <Textarea
              id="description"
              v-model="currentPoi.description"
              rows="3"
              class="w-full"
            />
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div class="field">
              <label for="latitude" class="block font-medium mb-1">纬度</label>
              <InputNumber
                id="latitude"
                v-model="currentPoi.latitude"
                :minFractionDigits="6"
                :maxFractionDigits="6"
                class="w-full"
              />
            </div>
            <div class="field">
              <label for="longitude" class="block font-medium mb-1">经度</label>
              <InputNumber
                id="longitude"
                v-model="currentPoi.longitude"
                :minFractionDigits="6"
                :maxFractionDigits="6"
                class="w-full"
              />
            </div>
          </div>

          <!-- 地图选点提示 -->
          <div v-if="!isEdit" class="bg-blue-50 p-3 rounded border-l-4 border-blue-400">
            <div class="flex items-center">
              <i class="i-ri-information-line text-blue-400 mr-2"></i>
              <p class="text-sm text-blue-700">
                提示：点击右侧地图选择布防点位置，坐标将自动填入
              </p>
            </div>
          </div>
        </div>

        <!-- 右侧：地图选择 -->
        <div class="space-y-2">
          <label class="block font-medium">位置选择</label>
          <div class="h-[350px] border rounded">
            <Tmap
              :center="dialogMapCenter"
              :zoom="14"
              :clickable="!isEdit"
              :temp-marker="tempMarkerPosition"
              @map-click="handleDialogMapClick"
            >
              <!-- 如果是编辑模式，显示当前布防点位置 -->
              <LMarker
                v-if="isEdit && currentPoi.latitude && currentPoi.longitude"
                :lat-lng="[currentPoi.latitude, currentPoi.longitude]"
              >
                <LPopup>
                  <div class="text-center">
                    <p class="font-bold">{{ currentPoi.name }}</p>
                    <p class="text-sm">当前位置</p>
                  </div>
                </LPopup>
              </LMarker>
              
              <!-- 调试信息：显示是否有临时标记 -->
              <!-- <div v-if="tempMarkerPosition" class="absolute top-2 left-2 bg-white p-2 rounded shadow z-[1000]">
                调试: 临时标记 {{ tempMarkerPosition }}
              </div> -->
            </Tmap>
          </div>
          <p class="text-xs text-gray-500" v-if="!isEdit">
            点击地图上的任意位置来设置布防点坐标
          </p>
          <!-- 添加调试信息显示 -->
          <p class="text-xs text-blue-500" v-if="tempMarkerPosition">
            已选位置: {{ tempMarkerPosition[0].toFixed(6) }}, {{ tempMarkerPosition[1].toFixed(6) }}
          </p>
        </div>
      </div>

      <template #footer>
        <Button
          label="取消"
          icon="i-ri-close-line"
          text
          @click="showDialog = false"
        />
        <Button label="保存" icon="i-ri-save-line" @click="savePoi" />
      </template>
    </Dialog>
  </div>
</template>
