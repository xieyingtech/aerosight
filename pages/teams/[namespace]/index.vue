<script setup lang="ts">
definePageMeta({
  layout: team,
});

const route = useRoute();
const namespace = route.params.namespace as string;

// 获取团队详情
const {
  data: team,
  pending,
  error,
} = await useFetch(`/api/teams/${namespace}`, {
  key: `team-${namespace}`,
});

// 创建项目对话框
const showCreateProjectDialog = ref(false);

// 项目表单
const projectForm = reactive({
  name: "",
  description: "",
});

// 表单验证
const projectFormErrors = ref({
  name: "",
  description: "",
});

// 验证项目表单
function validateProjectForm() {
  let isValid = true;
  projectFormErrors.value = { name: "", description: "" };

  if (!projectForm.name.trim()) {
    projectFormErrors.value.name = "请输入项目名称";
    isValid = false;
  }

  return isValid;
}

// 创建项目
async function createProject() {
  if (!validateProjectForm()) return false;

  try {
    await $fetch(`/api/teams/${namespace}/projects`, {
      method: "POST",
      body: projectForm,
    });

    // 刷新数据
    await refreshNuxtData(`team-${namespace}`);

    // 重置表单
    projectForm.name = "";
    projectForm.description = "";

    // 关闭对话框
    showCreateProjectDialog.value = false;

    return true;
  } catch (error: any) {
    // 处理错误
    console.error("创建项目失败", error);
    return false;
  }
}
</script>

<template>
  <div class="p-4">
    <div class="flex justify-between items-center mb-4">
      <h1 class="text-2xl font-bold">项目列表</h1>
      <Button
        label="创建项目"
        icon="i-ri-add-line"
        @click="showCreateProjectDialog = true"
      />
    </div>

    <!-- 加载状态 -->
    <div v-if="pending" class="p-4">
      <Skeleton :rows="5" animation="wave" />
    </div>

    <!-- 错误状态 -->
    <Message v-else-if="error" severity="error" :closable="false">
      {{ error.message }}
    </Message>

    <!-- 项目列表 - 使用Card网格布局 -->
    <div v-else-if="team && team.projects">
      <!-- 网格布局的项目卡片 -->
      <div v-if="team.projects.length > 0" class="grid">
        <div
          v-for="project in team.projects"
          :key="project.id"
          class="col-12 md:col-6 lg:col-4 xl:col-3 p-2"
        >
          <Card class="h-full">
            <template #header>
              <div class="flex justify-between items-center">
                <Badge
                  :value="project.poi?.length || 0"
                  severity="info"
                  class="mr-2"
                ></Badge>
                <div class="flex gap-1">
                  <Button
                    icon="i-ri-edit-line"
                    text
                    rounded
                    aria-label="编辑"
                    @click="
                      navigateTo(
                        `/teams/${namespace}/projects/${project.id}/edit`
                      )"
                  />
                  <Button
                    icon="i-ri-delete-bin-line"
                    text
                    rounded
                    severity="danger"
                    aria-label="删除"
                  />
                </div>
              </div>
            </template>

            <template #title>
              <div
                class="cursor-pointer"
                @click="navigateTo(`/teams/${namespace}/projects/${project.id}/poi`)"
              >
                {{ project.name }}
              </div>
            </template>

            <template #content>
              <p class="m-0 text-gray-600 line-clamp-3">
                {{ project.description || "暂无描述" }}
              </p>
            </template>

            <template #footer>
              <div class="flex justify-end pt-3">
                <Button
                  label="查看布防点"
                  icon="i-ri-map-pin-line"
                  outlined
                  size="small"
                  @click="navigateTo(`/teams/${namespace}/projects/${project.id}/poi`)"
                />
              </div>
            </template>
          </Card>
        </div>
      </div>

      <!-- 无项目状态 -->
      <div
        v-else
        class="text-center p-8 bg-gray-50 rounded-md mt-4"
      >
        <i class="i-ri-folder-line text-5xl text-gray-400 mb-3"></i>
        <p class="text-gray-500">暂无项目，点击上方"创建项目"按钮开始</p>
      </div>
    </div>

    <!-- 创建项目对话框 -->
    <Dialog
      v-model:visible="showCreateProjectDialog"
      modal
      header="创建新项目"
      :style="{ width: '30rem' }"
    >
      <div class="flex flex-col gap-4">
        <div class="flex flex-col gap-1">
          <label for="projectName" class="font-medium">项目名称</label>
          <InputText
            id="projectName"
            v-model="projectForm.name"
            placeholder="输入项目名称"
            :class="{ 'p-invalid': projectFormErrors.name }"
          />
          <small
            v-if="projectFormErrors.name"
            class="p-error"
          >{{ projectFormErrors.name }}</small>
        </div>

        <div class="flex flex-col gap-1">
          <label for="projectDescription" class="font-medium">项目描述</label>
          <Textarea
            id="projectDescription"
            v-model="projectForm.description"
            placeholder="请输入项目描述"
            rows="3"
            autoResize
          />
        </div>
      </div>

      <template #footer>
        <Button
          label="取消"
          text
          @click="showCreateProjectDialog = false"
        />
        <Button
          label="创建"
          icon="i-ri-check-line"
          @click="createProject"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.line-clamp-3 {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
