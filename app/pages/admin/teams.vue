<script setup lang="ts">
const { user } = useUserSession();
const { data, refresh } = useFetch("/api/admin/teams");

// 弹窗控制
const dialogVisible = ref(false);
const editDialogVisible = ref(false);

// 表单数据
const form = reactive({
  name: "",
  description: "",
});

// 表单验证状态
const submitted = ref(false);

// 提交表单
const submitForm = async () => {
  submitted.value = true;

  if (!form.name) {
    return;
  }

  try {
    await $fetch("/api/admin/teams", {
      method: "POST",
      body: form,
    });

    dialogVisible.value = false;
    // 重置表单
    form.name = "";
    form.description = "";
    submitted.value = false;
    // 刷新数据
    refresh();
  } catch (error) {
    console.error(error);
  }
};

const currentTeam = ref<any>(null);

const editTeam = (team: any) => {
  currentTeam.value = team;
  form.name = team.name;
  form.description = team.description || "";
  editDialogVisible.value = true;
  submitted.value = false;
};

const updateTeam = async () => {
  submitted.value = true;

  if (!form.name) {
    return;
  }

  try {
    await $fetch(`/api/admin/teams/${currentTeam.value?.id}`, {
      method: "PUT",
      body: form,
    });

    editDialogVisible.value = false;
    form.name = "";
    form.description = "";
    submitted.value = false;
    refresh();
  } catch (error) {
    console.error(error);
  }
};

const handleDelete = async (team: any) => {
  if (confirm(`确定要删除团队 "${team.name}" 吗？此操作不可恢复。`)) {
    try {
      await $fetch(`/api/admin/teams/${team.id}`, {
        method: "DELETE",
      });
      refresh();
    } catch (error) {
      console.error(error);
    }
  }
};

definePageMeta({ layout: "admin" });
</script>

<template>
  <div class="space-y-4">
    <UButton label="创建团队" icon="i-ri-add-line" @click="dialogVisible = true" />
    
    <UTable :rows="data ?? []" :loading="!data">
      <template #id-data="{ row }">
        {{ row.id }}
      </template>
      <template #name-data="{ row }">
        {{ row.name }}
      </template>
      <template #actions-data="{ row }">
        <div class="flex gap-2">
          <UButton
            icon="i-ri-pencil-line"
            variant="ghost"
            size="sm"
            @click="editTeam(row)"
          />
          <UButton
            icon="i-ri-delete-bin-line"
            variant="ghost"
            size="sm"
            color="error"
            @click="handleDelete(row)"
          />
        </div>
      </template>
    </UTable>

    <!-- 创建团队对话框 -->
    <UModal v-model="dialogVisible">
      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <h3 class="text-lg font-semibold">创建团队</h3>
            <UButton 
              color="neutral" 
              variant="ghost" 
              icon="i-ri-close-line" 
              @click="dialogVisible = false" 
            />
          </div>
        </template>

        <div class="space-y-4">
          <UFormField label="团队名称" name="name" :error="submitted && !form.name ? '团队名称不能为空' : ''">
            <UInput v-model="form.name" placeholder="请输入团队名称" />
          </UFormField>

          <UFormField label="团队描述" name="description">
            <UTextarea v-model="form.description" placeholder="请输入团队描述" :rows="3" />
          </UFormField>
        </div>

        <template #footer>
          <div class="flex justify-end gap-3">
            <UButton
              color="neutral"
              variant="ghost"
              @click="dialogVisible = false"
            >
              取消
            </UButton>
            <UButton @click="submitForm">
              创建
            </UButton>
          </div>
        </template>
      </UCard>
    </UModal>

    <!-- 编辑团队对话框 -->
    <UModal v-model="editDialogVisible">
      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <h3 class="text-lg font-semibold">编辑团队</h3>
            <UButton 
              color="neutral" 
              variant="ghost" 
              icon="i-ri-close-line" 
              @click="editDialogVisible = false" 
            />
          </div>
        </template>

        <div class="space-y-4">
          <UFormField label="团队名称" name="name" :error="submitted && !form.name ? '团队名称不能为空' : ''">
            <UInput v-model="form.name" placeholder="请输入团队名称" />
          </UFormField>

          <UFormField label="团队描述" name="description">
            <UTextarea v-model="form.description" placeholder="请输入团队描述" :rows="3" />
          </UFormField>
        </div>

        <template #footer>
          <div class="flex justify-end gap-3">
            <UButton
              color="neutral"
              variant="ghost"
              @click="editDialogVisible = false"
            >
              取消
            </UButton>
            <UButton @click="updateTeam">
              保存
            </UButton>
          </div>
        </template>
      </UCard>
    </UModal>
  </div>
</template>
