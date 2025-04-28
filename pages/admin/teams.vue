<script setup lang="ts">
const { user } = useUserSession();
const { data, refresh } = useFetch("/api/admin/teams");

// 弹窗控制
const dialogVisible = ref(false);

// 表单数据
const form = reactive({
  name: "",
  description: "",
});

// 表单规则
const rules = {
  name: [{ required: true, message: "组织名称不能为空", trigger: "blur" }],
};

const formRef = ref();

// 提交表单
const submitForm = async () => {
  if (!formRef.value) return;

  await formRef.value.validate(async (valid: boolean) => {
    if (valid) {
      try {
        await $fetch("/api/admin/teams", {
          method: "POST",
          body: form,
        });

        ElMessage.success("组织创建成功");
        dialogVisible.value = false;

        // 重置表单
        form.name = "";
        form.description = "";

        // 刷新数据
        refresh();
      } catch (error) {
        ElMessage.error("创建组织失败");
        console.error(error);
      }
    }
  });
};

const currentTeam = ref(null);
const editDialogVisible = ref(false);

const editTeam = (team) => {
  currentTeam.value = team;
  form.name = team.name;
  form.description = team.description || "";
  editDialogVisible.value = true;
};

const updateTeam = async () => {
  if (!formRef.value) return;

  await formRef.value.validate(async (valid: boolean) => {
    if (valid) {
      try {
        await $fetch(`/api/admin/teams/${currentTeam.value.id}`, {
          method: "PUT",
          body: form,
        });

        ElMessage.success("更新成功");
        editDialogVisible.value = false;
        form.name = "";
        form.description = "";
        refresh();
      } catch (error) {
        ElMessage.error("更新失败");
        console.error(error);
      }
    }
  });
};

const handleDelete = async (team) => {
  try {
    await ElMessageBox.confirm(
      '确定要删除组织 "' + team.name + '" 吗？此操作不可恢复。',
      "删除确认",
      {
        confirmButtonText: "确定",
        cancelButtonText: "取消",
        type: "warning",
      }
    );

    await $fetch(`/api/admin/teams/${team.id}`, {
      method: "DELETE",
    });

    ElMessage.success("删除成功");
    refresh();
  } catch (error) {
    if (error !== "cancel") {
      ElMessage.error("删除失败");
      console.error(error);
    }
  }
};

definePageMeta({ layout: "admin" });
</script>

<template>
  <ElPageHeader title="组织管理" icon="">
    <template #extra>
      <ElButton type="primary" @click="dialogVisible = true">
        <i class="el-icon i-ri-add-line mr-1"></i> 创建组织
      </ElButton>
    </template>
  </ElPageHeader>
  <ElTable :data="data ?? []">
    <ElTableColumn prop="id" label="ID" />
    <ElTableColumn prop="name" label="名称" />
    <ElTableColumn label="操作">
      <template #default="scope">
        <ElButton size="small" type="primary" text @click="editTeam(scope.row)">
          编辑</ElButton
        >
        <ElButton
          size="small"
          type="danger"
          text
          @click="handleDelete(scope.row)"
          >删除</ElButton
        >
      </template>
    </ElTableColumn>
  </ElTable>
  <ElDialog v-model="dialogVisible" title="创建组织">
    <ElForm ref="formRef" :model="form" :rules="rules" label-position="top">
      <ElFormItem label="组织名称" prop="name">
        <ElInput v-model="form.name" placeholder="请输入组织名称" />
      </ElFormItem>
      <ElFormItem label="组织描述" prop="description">
        <ElInput
          v-model="form.description"
          type="textarea"
          placeholder="请输入组织描述"
        />
      </ElFormItem>
    </ElForm>
    <template #footer>
      <div class="flex justify-end gap-2">
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton type="primary" @click="submitForm">创建</ElButton>
      </div>
    </template>
  </ElDialog>
  <ElDialog v-model="editDialogVisible" title="编辑组织">
    <ElForm ref="formRef" :model="form" :rules="rules" label-position="top">
      <ElFormItem label="组织名称" prop="name">
        <ElInput v-model="form.name" placeholder="请输入组织名称" />
      </ElFormItem>
      <ElFormItem label="组织描述" prop="description">
        <ElInput
          v-model="form.description"
          type="textarea"
          placeholder="请输入组织描述"
        />
      </ElFormItem>
    </ElForm>
    <template #footer>
      <ElButton @click="editDialogVisible = false">取消</ElButton>
      <ElButton type="primary" @click="updateTeam">保存</ElButton>
    </template>
  </ElDialog>
</template>
