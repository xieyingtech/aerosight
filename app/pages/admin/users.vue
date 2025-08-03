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
  systemAdmin: false,
});

// 表单验证状态
const formIsValid = ref(false);
const submitted = ref(false);

// 提交表单
const submitForm = async () => {
  submitted.value = true;

  if (!form.username || !form.password) {
    return;
  }

  try {
    await $fetch("/api/admin/users", {
      method: "POST",
      body: form,
    });

    dialogVisible.value = false;
    // 重置表单
    resetForm();
    // 刷新数据
    refresh();
  } catch (error) {
    console.error(error);
  }
};

const resetForm = () => {
  form.username = "";
  form.email = "";
  form.phone = "";
  form.password = "";
  form.systemAdmin = false;
  submitted.value = false;
};

const currentUser = ref<any>(null);

const editUser = (user: any) => {
  currentUser.value = user;
  form.username = user.username;
  form.email = user.email || "";
  form.phone = user.phone || "";
  form.systemAdmin = user.systemAdmin || false;
  form.password = ""; // 密码不回填
  editDialogVisible.value = true;
  submitted.value = false;
};

const updateUser = async () => {
  submitted.value = true;

  if (!form.username) {
    return;
  }

  try {
    const updateData: any = { ...form };
    // 如果密码为空，不更新密码
    if (!updateData.password) {
      delete updateData.password;
    }

    await $fetch(`/api/admin/users/${currentUser.value?.id}`, {
      method: "PUT",
      body: updateData,
    });

    editDialogVisible.value = false;
    resetForm();
    refresh();
  } catch (error) {
    console.error(error);
  }
};

const handleDelete = async (user: any) => {
  if (confirm(`确定要删除用户 "${user.username}" 吗？此操作不可恢复。`)) {
    try {
      await $fetch(`/api/admin/users/${user.id}`, {
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
    <UButton label="创建用户" icon="i-ri-add-line" @click="dialogVisible = true" />
    
    <UTable :rows="data ?? []" :loading="!data && !error">
      <template #id-data="{ row }">
        {{ row.id }}
      </template>
      <template #username-data="{ row }">
        {{ row.username }}
      </template>
      <template #email-data="{ row }">
        {{ row.email }}
      </template>
      <template #phone-data="{ row }">
        {{ row.phone }}
      </template>
      <template #systemAdmin-data="{ row }">
        <UBadge :color="row.systemAdmin ? 'green' : 'blue'">
          {{ row.systemAdmin ? "是" : "否" }}
        </UBadge>
      </template>
      <template #actions-data="{ row }">
        <div class="flex gap-2">
          <UButton
            icon="i-ri-pencil-line"
            variant="ghost"
            size="sm"
            @click="editUser(row)"
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

    <!-- 创建用户对话框 -->
    <UModal v-model="dialogVisible">
      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <h3 class="text-lg font-semibold">创建用户</h3>
            <UButton 
              color="neutral" 
              variant="ghost" 
              icon="i-ri-close-line" 
              @click="dialogVisible = false" 
            />
          </div>
        </template>

        <div class="space-y-4">
          <UFormField label="用户名" name="username" :error="submitted && !form.username ? '用户名不能为空' : ''">
            <UInput v-model="form.username" placeholder="请输入用户名" />
          </UFormField>

          <UFormField label="密码" name="password" :error="submitted && !form.password ? '密码不能为空' : ''">
            <UInput v-model="form.password" type="password" placeholder="请输入密码" />
          </UFormField>

          <UFormField label="邮箱" name="email">
            <UInput v-model="form.email" type="email" placeholder="请输入邮箱" />
          </UFormField>

          <UFormField label="电话" name="phone">
            <UInput v-model="form.phone" placeholder="请输入电话" />
          </UFormField>

          <UFormField label="系统管理员" name="systemAdmin">
            <UToggle v-model="form.systemAdmin" />
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

    <!-- 编辑用户对话框 -->
    <UModal v-model="editDialogVisible">
      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <h3 class="text-lg font-semibold">编辑用户</h3>
            <UButton 
              color="neutral" 
              variant="ghost" 
              icon="i-ri-close-line" 
              @click="editDialogVisible = false" 
            />
          </div>
        </template>

        <div class="space-y-4">
          <UFormField label="用户名" name="username" :error="submitted && !form.username ? '用户名不能为空' : ''">
            <UInput v-model="form.username" placeholder="请输入用户名" />
          </UFormField>

          <UFormField label="密码" name="password">
            <UInput v-model="form.password" type="password" placeholder="留空表示不修改密码" />
          </UFormField>

          <UFormField label="邮箱" name="email">
            <UInput v-model="form.email" type="email" placeholder="请输入邮箱" />
          </UFormField>

          <UFormField label="电话" name="phone">
            <UInput v-model="form.phone" placeholder="请输入电话" />
          </UFormField>

          <UFormField label="系统管理员" name="systemAdmin">
            <UToggle v-model="form.systemAdmin" />
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
            <UButton @click="updateUser">
              保存
            </UButton>
          </div>
        </template>
      </UCard>
    </UModal>
  </div>
</template>
