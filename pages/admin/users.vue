<script setup lang="ts">
const { data, error, refresh } = useFetch("/api/admin/users");

// 弹窗控制
const dialogVisible = ref(false);
const editDialogVisible = ref(false);

// 表单数据
const form = reactive({
  username: "",
  email: "",
  phone: "",
  password: "",
  admin: false
});

// 表单规则
const rules = {
  username: [{ required: true, message: "用户名不能为空", trigger: "blur" }],
  password: [{ required: true, message: "密码不能为空", trigger: "blur" }],
  email: [{ type: "email", message: "请输入正确的邮箱格式", trigger: "blur" }]
};

const formRef = ref();

// 提交表单
const submitForm = async () => {
  if (!formRef.value) return;

  await formRef.value.validate(async (valid: boolean) => {
    if (valid) {
      try {
        await $fetch("/api/admin/users", {
          method: "POST",
          body: form,
        });

        ElMessage.success("用户创建成功");
        dialogVisible.value = false;

        // 重置表单
        resetForm();

        // 刷新数据
        refresh();
      } catch (error) {
        ElMessage.error("创建用户失败");
        console.error(error);
      }
    }
  });
};

const resetForm = () => {
  form.username = "";
  form.email = "";
  form.phone = "";
  form.password = "";
  form.admin = false;
};

const currentUser = ref(null);

const editUser = (user) => {
  currentUser.value = user;
  form.username = user.username;
  form.email = user.email || "";
  form.phone = user.phone || "";
  form.admin = user.admin || false;
  form.password = ""; // 密码不回填
  editDialogVisible.value = true;
};

const updateUser = async () => {
  if (!formRef.value) return;

  await formRef.value.validate(async (valid: boolean) => {
    if (valid) {
      try {
        const updateData = { ...form };
        // 如果密码为空，不更新密码
        if (!updateData.password) {
          delete updateData.password;
        }

        await $fetch(`/api/admin/users/${currentUser.value.id}`, {
          method: "PUT",
          body: updateData,
        });

        ElMessage.success("更新成功");
        editDialogVisible.value = false;
        resetForm();
        refresh();
      } catch (error) {
        ElMessage.error("更新失败");
        console.error(error);
      }
    }
  });
};

const handleDelete = async (user) => {
  try {
    await ElMessageBox.confirm(
      '确定要删除用户 "' + user.username + '" 吗？此操作不可恢复。',
      "删除确认",
      {
        confirmButtonText: "确定",
        cancelButtonText: "取消",
        type: "warning",
      }
    );

    await $fetch(`/api/admin/users/${user.id}`, {
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
  <ElPageHeader title="用户管理" icon="">
    <template #extra>
      <ElButton type="primary" @click="dialogVisible = true">
        <i class="el-icon i-ri-add-line mr-1"></i> 创建用户
      </ElButton>
    </template>
  </ElPageHeader>
  <ElTable :data="data ?? []">
    <ElTableColumn prop="id" label="ID" />
    <ElTableColumn prop="username" label="用户名" />
    <ElTableColumn prop="email" label="邮箱" />
    <ElTableColumn prop="phone" label="电话" />
    <ElTableColumn prop="admin" label="管理员">
      <template #default="scope">
        <ElTag :type="scope.row.admin ? 'success' : 'info'">
          {{ scope.row.admin ? '是' : '否' }}
        </ElTag>
      </template>
    </ElTableColumn>
    <ElTableColumn label="操作">
      <template #default="scope">
        <ElButton size="small" type="primary" text @click="editUser(scope.row)">
          编辑
        </ElButton>
        <ElButton
          size="small"
          type="danger"
          text
          @click="handleDelete(scope.row)"
        >
          删除
        </ElButton>
      </template>
    </ElTableColumn>
  </ElTable>

  <!-- 创建用户对话框 -->
  <ElDialog v-model="dialogVisible" title="创建用户">
    <ElForm ref="formRef" :model="form" :rules="rules" label-position="top">
      <ElFormItem label="用户名" prop="username">
        <ElInput v-model="form.username" placeholder="请输入用户名" />
      </ElFormItem>
      <ElFormItem label="密码" prop="password">
        <ElInput v-model="form.password" type="password" placeholder="请输入密码" />
      </ElFormItem>
      <ElFormItem label="邮箱" prop="email">
        <ElInput v-model="form.email" placeholder="请输入邮箱" />
      </ElFormItem>
      <ElFormItem label="电话" prop="phone">
        <ElInput v-model="form.phone" placeholder="请输入电话" />
      </ElFormItem>
      <ElFormItem label="管理员">
        <ElSwitch v-model="form.admin" />
      </ElFormItem>
    </ElForm>
    <template #footer>
      <div class="flex justify-end gap-2">
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton type="primary" @click="submitForm">创建</ElButton>
      </div>
    </template>
  </ElDialog>

  <!-- 编辑用户对话框 -->
  <ElDialog v-model="editDialogVisible" title="编辑用户">
    <ElForm ref="formRef" :model="form" :rules="rules" label-position="top">
      <ElFormItem label="用户名" prop="username">
        <ElInput v-model="form.username" placeholder="请输入用户名" />
      </ElFormItem>
      <ElFormItem label="密码">
        <ElInput v-model="form.password" type="password" placeholder="留空表示不修改密码" />
      </ElFormItem>
      <ElFormItem label="邮箱" prop="email">
        <ElInput v-model="form.email" placeholder="请输入邮箱" />
      </ElFormItem>
      <ElFormItem label="电话" prop="phone">
        <ElInput v-model="form.phone" placeholder="请输入电话" />
      </ElFormItem>
      <ElFormItem label="管理员">
        <ElSwitch v-model="form.admin" />
      </ElFormItem>
    </ElForm>
    <template #footer>
      <ElButton @click="editDialogVisible = false">取消</ElButton>
      <ElButton type="primary" @click="updateUser">保存</ElButton>
    </template>
  </ElDialog>
</template>

