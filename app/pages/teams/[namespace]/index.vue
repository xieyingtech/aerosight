<script setup lang="ts">
definePageMeta({
  layout: "team",
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
      <UButton label="创建项目" icon="i-ri-add-line" @click="showCreateProjectDialog = true" />
    </div>

    <!-- 加载状态 -->
    <div v-if="pending" class="p-4">
      <USkeleton :rows="5" animation="wave" />
    </div>

    <!-- 错误状态 -->
    <UAlert v-else-if="error" color="red" icon="i-ri-error-warning-line" :closable="false">
      {{ error.message }}
    </UAlert>

    <!-- 项目列表 - 使用UCard网格布局 -->
    <div v-else-if="team && team.projects">
      <!-- 网格布局的项目卡片 -->
      <UGrid v-if="team.projects.length > 0" cols="1 md:2 lg:3 xl:4" gap="4">
        <UCard v-for="project in team.projects" :key="project.id" class="h-full">
          <template #header>
            <div class="flex justify-between items-center">
              <UBadge color="primary" class="mr-2">
                {{ project.poi?.length || 0 }}
              </UBadge>
              <div class="flex gap-1">
                <UButton
                  icon="i-ri-edit-line"
                  variant="ghost"
                  size="sm"
                  aria-label="编辑"
                  @click="navigateTo(`/teams/${namespace}/projects/${project.id}/edit`)"
                />
                <UButton
                  icon="i-ri-delete-bin-line"
                  variant="ghost"
                  size="sm"
                  color="error"
                  aria-label="删除"
                />
              </div>
            </div>
          </template>
          <template #title>
            <div
              class="cursor-pointer"
              @click="navigateTo(`/teams/${namespace}/projects/${project.id}`)"
            >
              {{ project.name }}
            </div>
          </template>
          <template #content>
            <p class="m-0 text-gray-600 dark:text-gray-300 line-clamp-3">
              {{ project.description || "暂无描述" }}
            </p>
          </template>
          <template #footer>
            <div class="flex justify-end pt-3">
              <UButton
                label="查看项目"
                icon="i-ri-folder-open-line"
                variant="outline"
                size="sm"
                @click="navigateTo(`/teams/${namespace}/projects/${project.id}`)"
              />
            </div>
          </template>
        </UCard>
      </UGrid>

      <!-- 无项目状态 -->
      <div v-else class="text-center p-8 bg-gray-50 dark:bg-gray-900 rounded-md mt-4">
        <UIcon name="i-ri-folder-line" class="text-5xl text-gray-400 mb-3" />
        <p class="text-gray-500 dark:text-gray-300">暂无项目，点击上方"创建项目"按钮开始</p>
      </div>
    </div>

    <!-- 创建项目对话框 -->
    <UModal v-model="showCreateProjectDialog">
      <UCard>
        <template #header>
          <span class="font-bold">创建新项目</span>
        </template>
        <div class="space-y-4">
          <UFormField label="项目名称" name="name" :error="projectFormErrors.name">
            <UInput v-model="projectForm.name" placeholder="输入项目名称" />
          </UFormField>
          <UFormField label="项目描述" name="description">
            <UTextarea v-model="projectForm.description" placeholder="请输入项目描述" :rows="3" />
          </UFormField>
        </div>
        <template #footer>
          <div class="flex justify-end gap-3">
            <UButton color="neutral" variant="ghost" @click="showCreateProjectDialog = false">取消</UButton>
            <UButton icon="i-ri-check-line" @click="createProject">创建</UButton>
          </div>
        </template>
      </UCard>
    </UModal>
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
