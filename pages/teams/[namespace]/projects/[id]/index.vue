<script setup lang="ts">
definePageMeta({
  layout: team,
});

const route = useRoute();
const namespace = route.params.namespace as string;
const projectId = parseInt(route.params.id as string);
const toast = useToast();

// 获取项目详情和布防点
const { data: project, refresh: refreshProject } = await useFetch(
  `/api/teams/${namespace}/projects/${projectId}`,
  { key: `project-${projectId}` }
);

// 布防点表单对话框
const poiDialog = ref(false);
const dialogMode = ref('create'); // 'create' 或 'edit'
const selectedPoi = ref(null);

// 布防点表单
const poiForm = reactive({
  id: null,
  name: '',
  description: '',
  latitude: 0,
  longitude: 0
});

// 表单错误
const poiFormErrors = ref({
  name: '',
  latitude: '',
  longitude: ''
});

// 验证表单
function validatePoiForm() {
  let isValid = true;
  poiFormErrors.value = { name: '', latitude: '', longitude: '' };

  if (!poiForm.name.trim()) {
    poiFormErrors.value.name = '请输入布防点名称';
    isValid = false;
  }

  if (!poiForm.latitude) {
    poiFormErrors.value.latitude = '请输入纬度';
    isValid = false;
  }

  if (!poiForm.longitude) {
    poiFormErrors.value.longitude = '请输入经度';
    isValid = false;
  }

  return isValid;
}

// 重置表单
function resetPoiForm() {
  poiForm.id = null;
  poiForm.name = '';
  poiForm.description = '';
  poiForm.latitude = 0;
  poiForm.longitude = 0;
  poiFormErrors.value = { name: '', latitude: '', longitude: '' };
}

// 打开创建布防点对话框
function openCreatePoiDialog(latlng = null) {
  resetPoiForm();
  dialogMode.value = 'create';
  
  // 如果有坐标，填入表单
  if (latlng) {
    poiForm.latitude = latlng.lat;
    poiForm.longitude = latlng.lng;
  }
  
  poiDialog.value = true;
}

// 打开编辑布防点对话框
function openEditPoiDialog(poi) {
  resetPoiForm();
  dialogMode.value = 'edit';
  selectedPoi.value = poi;
  
  poiForm.id = poi.id;
  poiForm.name = poi.name;
  poiForm.description = poi.description || '';
  poiForm.latitude = poi.latitude;
  poiForm.longitude = poi.longitude;
  
  poiDialog.value = true;
}

// 提交布防点表单
async function submitPoiForm() {
  if (!validatePoiForm()) return false;
  
  try {
    if (dialogMode.value === 'create') {
      // 创建新布防点
      await $fetch(`/api/teams/${namespace}/projects/${projectId}/poi`, {
        method: 'POST',
        body: {
          name: poiForm.name,
          description: poiForm.description,
          latitude: poiForm.latitude,
          longitude: poiForm.longitude
        }
      });
      
      toast.add({
        severity: 'success',
        summary: '成功',
        detail: '布防点创建成功',
        life: 3000
      });
    } else {
      // 更新布防点
      await $fetch(`/api/teams/${namespace}/projects/${projectId}/poi/${poiForm.id}`, {
        method: 'PUT',
        body: {
          name: poiForm.name,
          description: poiForm.description,
          latitude: poiForm.latitude,
          longitude: poiForm.longitude
        }
      });
      
      toast.add({
        severity: 'success',
        summary: '成功',
        detail: '布防点更新成功',
        life: 3000
      });
    }
    
    // 刷新数据
    await refreshProject();
    
    // 关闭对话框
    poiDialog.value = false;
    return true;
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: '错误',
      detail: error.message || '操作失败',
      life: 3000
    });
    return false;
  }
}

// 删除布防点
async function deletePoi(poi) {
  try {
    await $fetch(`/api/teams/${namespace}/projects/${projectId}/poi/${poi.id}`, {
      method: 'DELETE'
    });
    
    toast.add({
      severity: 'success',
      summary: '成功',
      detail: '布防点删除成功',
      life: 3000
    });
    
    // 刷新数据
    await refreshProject();
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: '错误',
      detail: error.message || '删除失败',
      life: 3000
    });
  }
}

// 确认删除
function confirmDeletePoi(poi) {
  confirm.require({
    message: `确定要删除"${poi.name}"吗？`,
    header: '删除布防点',
    icon: 'i-ri-delete-bin-line',
    acceptClass: 'p-button-danger',
    accept: () => deletePoi(poi),
    reject: () => {}
  });
}

// 地图点击事件处理
function handleMapClick(e) {
  openCreatePoiDialog(e.latlng);
}

// 计算地图中心点
const mapCenter = computed(() => {
  if (!project.value || !project.value.poi || project.value.poi.length === 0) {
    return [39.906217, 116.3912757]; // 默认位置：北京
  }
  
  // 计算所有布防点的平均坐标
  const sum = project.value.poi.reduce(
    (acc, poi) => {
      acc.lat += poi.latitude;
      acc.lng += poi.longitude;
      return acc;
    },
    { lat: 0, lng: 0 }
  );
  
  return [
    sum.lat / project.value.poi.length,
    sum.lng / project.value.poi.length
  ];
});

const confirm = useConfirm();
</script>

<template>
  <div class="project-container h-full flex flex-col">
    <!-- 项目标题和工具栏 -->
    <div class="flex justify-between items-center mb-2">
      <div>
        <h1 class="text-xl font-bold mb-0" v-if="project">{{ project.name }}</h1>
        <p class="text-gray-500 text-sm" v-if="project">{{ project.description }}</p>
      </div>
      <div>
        <Button 
          label="添加布防点" 
          icon="i-ri-map-pin-add-line" 
          class="mr-2"
          @click="openCreatePoiDialog()"
        />
        <Button 
          label="刷新" 
          icon="i-ri-refresh-line"
          outlined
          @click="refreshProject()"
        />
      </div>
    </div>
    
    <!-- 地图和布防点列表 -->
    <div class="grid flex-grow">
      <!-- 地图 -->
      <div class="col-12 md:col-8">
        <div class="map-container h-full border rounded overflow-hidden" style="min-height: 500px;">
          <Tmap 
            :center="mapCenter" 
            :zoom="13"
            @click="handleMapClick"
          >
            <template v-if="project && project.poi">
              <LMarker 
                v-for="poi in project.poi" 
                :key="poi.id"
                :lat-lng="[poi.latitude, poi.longitude]"
              >
                <LPopup>
                  <div class="poi-popup">
                    <h3 class="text-lg font-bold">{{ poi.name }}</h3>
                    <p v-if="poi.description">{{ poi.description }}</p>
                    <div class="flex gap-2 mt-2">
                      <Button 
                        icon="i-ri-edit-line" 
                        text 
                        size="small"
                        @click="openEditPoiDialog(poi)"
                      />
                      <Button 
                        icon="i-ri-delete-bin-line" 
                        text 
                        severity="danger" 
                        size="small"
                        @click="confirmDeletePoi(poi)"
                      />
                    </div>
                  </div>
                </LPopup>
              </LMarker>
            </template>
          </Tmap>
        </div>
      </div>
      
      <!-- 布防点列表 -->
      <div class="col-12 md:col-4">
        <Card>
          <template #title>
            <div class="flex justify-between items-center">
              <span>布防点列表</span>
              <Badge :value="project?.poi?.length || 0" severity="info" />
            </div>
          </template>
          <template #content>
            <div v-if="!project?.poi || project.poi.length === 0" class="text-center py-4">
              <i class="i-ri-map-pin-line text-4xl text-gray-400"></i>
              <p class="text-gray-500">暂无布防点</p>
              <small class="text-gray-400">点击地图或使用"添加布防点"按钮创建</small>
            </div>
            <div v-else class="poi-list max-h-96 overflow-y-auto">
              <div 
                v-for="poi in project.poi" 
                :key="poi.id"
                class="poi-item p-3 border-b hover:bg-gray-50 cursor-pointer"
                @click="openEditPoiDialog(poi)"
              >
                <div class="flex justify-between">
                  <h3 class="font-bold">{{ poi.name }}</h3>
                  <div>
                    <Button 
                      icon="i-ri-delete-bin-line" 
                      text 
                      severity="danger" 
                      size="small"
                      @click.stop="confirmDeletePoi(poi)"
                    />
                  </div>
                </div>
                <p class="text-sm text-gray-600 line-clamp-1">{{ poi.description || '无描述' }}</p>
                <div class="text-xs text-gray-500 mt-1">
                  <span>{{ poi.latitude.toFixed(6) }}, {{ poi.longitude.toFixed(6) }}</span>
                </div>
              </div>
            </div>
          </template>
        </Card>
      </div>
    </div>
    
    <!-- 布防点表单对话框 -->
    <Dialog 
      v-model:visible="poiDialog" 
      :header="dialogMode === 'create' ? '添加布防点' : '编辑布防点'"
      modal
      :style="{ width: '30rem' }"
    >
      <div class="flex flex-col gap-4">
        <div class="flex flex-col gap-1">
          <label for="poiName" class="font-medium">名称</label>
          <InputText 
            id="poiName" 
            v-model="poiForm.name" 
            placeholder="输入布防点名称"
            :class="{ 'p-invalid': poiFormErrors.name }"
          />
          <small v-if="poiFormErrors.name" class="p-error">{{ poiFormErrors.name }}</small>
        </div>
        
        <div class="flex flex-col gap-1">
          <label for="poiDescription" class="font-medium">描述</label>
          <Textarea 
            id="poiDescription" 
            v-model="poiForm.description" 
            placeholder="输入描述信息"
            rows="3"
            autoResize
          />
        </div>
        
        <div class="grid">
          <div class="col-6 flex flex-col gap-1">
            <label for="poiLatitude" class="font-medium">纬度</label>
            <InputNumber 
              id="poiLatitude" 
              v-model="poiForm.latitude" 
              :min="-90" 
              :max="90" 
              :decimal-places="6"
              placeholder="纬度"
              :class="{ 'p-invalid': poiFormErrors.latitude }"
            />
            <small v-if="poiFormErrors.latitude" class="p-error">{{ poiFormErrors.latitude }}</small>
          </div>
          
          <div class="col-6 flex flex-col gap-1">
            <label for="poiLongitude" class="font-medium">经度</label>
            <InputNumber 
              id="poiLongitude" 
              v-model="poiForm.longitude" 
              :min="-180" 
              :max="180" 
              :decimal-places="6"
              placeholder="经度"
              :class="{ 'p-invalid': poiFormErrors.longitude }"
            />
            <small v-if="poiFormErrors.longitude" class="p-error">{{ poiFormErrors.longitude }}</small>
          </div>
        </div>
      </div>
      
      <template #footer>
        <Button 
          label="取消" 
          text 
          @click="poiDialog = false" 
        />
        <Button 
          :label="dialogMode === 'create' ? '创建' : '保存'"
          :icon="dialogMode === 'create' ? 'i-ri-add-line' : 'i-ri-save-line'"
          @click="submitPoiForm" 
        />
      </template>
    </Dialog>
    
    <ConfirmDialog />
  </div>
</template>

<style scoped>
.project-container {
  min-height: calc(100vh - 120px);
}

.line-clamp-1 {
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.poi-item:last-child {
  border-bottom: none;
}
</style>
