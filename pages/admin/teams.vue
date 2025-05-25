<script setup lang="ts">
const { user } = useUserSession();
const { data, refresh } = useFetch("/api/admin/teams");
const toast = useToast();
const confirmService = useConfirm();

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

    toast.add({
      severity: "success",
      summary: "成功",
      detail: "团队创建成功",
      life: 3000,
    });
    dialogVisible.value = false;

    // 重置表单
    form.name = "";
    form.description = "";
    submitted.value = false;

    // 刷新数据
    refresh();
  } catch (error) {
    toast.add({
      severity: "error",
      summary: "错误",
      detail: "创建团队失败",
      life: 3000,
    });
    console.error(error);
  }
};

const currentTeam = ref(null);

const editTeam = (team) => {
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
    await $fetch(`/api/admin/teams/${currentTeam.value.id}`, {
      method: "PUT",
      body: form,
    });

    toast.add({
      severity: "success",
      summary: "成功",
      detail: "更新成功",
      life: 3000,
    });
    editDialogVisible.value = false;
    form.name = "";
    form.description = "";
    submitted.value = false;
    refresh();
  } catch (error) {
    toast.add({
      severity: "error",
      summary: "错误",
      detail: "更新失败",
      life: 3000,
    });
    console.error(error);
  }
};

const handleDelete = async (team) => {
  confirmService.require({
    message: `确定要删除团队 "${team.name}" 吗？此操作不可恢复。`,
    header: "删除确认",
    icon: "pi pi-exclamation-triangle",
    acceptLabel: "确定",
    rejectLabel: "取消",
    accept: async () => {
      try {
        await $fetch(`/api/admin/teams/${team.id}`, {
          method: "DELETE",
        });

        toast.add({
          severity: "success",
          summary: "成功",
          detail: "删除成功",
          life: 3000,
        });
        refresh();
      } catch (error) {
        toast.add({
          severity: "error",
          summary: "错误",
          detail: "删除失败",
          life: 3000,
        });
        console.error(error);
      }
    },
  });
};

definePageMeta({ layout: "admin" });
</script>

<template>
  <Button label="创建团队" icon="i-ri-add-line" @click="dialogVisible = true" />
  <DataTable :value="data ?? []" stripedRows>
    <Column field="id" header="ID" />
    <Column field="name" header="名称" />
    <Column header="操作">
      <template #body="slotProps">
        <Button
          icon="i-ri-pencil-line"
          text
          @click="editTeam(slotProps.data)"
        />
        <Button
          icon="i-ri-delete-bin-line"
          text
          severity="danger"
          @click="handleDelete(slotProps.data)"
        />
      </template>
    </Column>
  </DataTable>

  <!-- 创建团队对话框 -->
  <Dialog
    v-model:visible="dialogVisible"
    header="创建团队"
    modal
    :style="{ width: '450px' }"
    :closable="true"
  >
    <div class="p-fluid">
      <div class="field mb-4">
        <label for="name" class="block mb-1">团队名称</label>
        <InputText
          id="name"
          v-model="form.name"
          :class="{ 'p-invalid': submitted && !form.name }"
        />
        <small v-if="submitted && !form.name" class="p-error"
          >团队名称不能为空</small
        >
      </div>

      <div class="field">
        <label for="description" class="block mb-1">团队描述</label>
        <Textarea
          id="description"
          v-model="form.description"
          rows="5"
          autoResize
        />
      </div>
    </div>

    <template #footer>
      <Button
        label="取消"
        icon="i-ri-close-line"
        text
        @click="dialogVisible = false"
      />
      <Button label="创建" icon="i-ri-check-line" @click="submitForm" />
    </template>
  </Dialog>

  <!-- 编辑团队对话框 -->
  <Dialog
    v-model:visible="editDialogVisible"
    header="编辑团队"
    modal
    :style="{ width: '450px' }"
    :closable="true"
  >
    <div class="p-fluid">
      <div class="field mb-4">
        <label for="edit-name" class="block mb-1">团队名称</label>
        <InputText
          id="edit-name"
          v-model="form.name"
          :class="{ 'p-invalid': submitted && !form.name }"
        />
        <small v-if="submitted && !form.name" class="p-error"
          >团队名称不能为空</small
        >
      </div>

      <div class="field">
        <label for="edit-description" class="block mb-1">团队描述</label>
        <Textarea
          id="edit-description"
          v-model="form.description"
          rows="5"
          autoResize
        />
      </div>
    </div>

    <template #footer>
      <Button
        label="取消"
        icon="i-ri-close-line"
        text
        @click="editDialogVisible = false"
      />
      <Button label="保存" icon="i-ri-check-line" @click="updateTeam" />
    </template>
  </Dialog>
</template>
