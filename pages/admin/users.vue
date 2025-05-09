<script setup lang="ts">
const { data, error, refresh } = useFetch("/api/admin/users");
const toast = useToast();
const confirmService = useConfirm();

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

    toast.add({ severity: 'success', summary: '成功', detail: '用户创建成功', life: 3000 });
    dialogVisible.value = false;

    // 重置表单
    resetForm();

    // 刷新数据
    refresh();
  } catch (error) {
    toast.add({ severity: 'error', summary: '错误', detail: '创建用户失败', life: 3000 });
    console.error(error);
  }
};

const resetForm = () => {
  form.username = "";
  form.email = "";
  form.phone = "";
  form.password = "";
  form.admin = false;
  submitted.value = false;
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
  submitted.value = false;
};

const updateUser = async () => {
  submitted.value = true;
  
  if (!form.username) {
    return;
  }

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

    toast.add({ severity: 'success', summary: '成功', detail: '更新成功', life: 3000 });
    editDialogVisible.value = false;
    resetForm();
    refresh();
  } catch (error) {
    toast.add({ severity: 'error', summary: '错误', detail: '更新失败', life: 3000 });
    console.error(error);
  }
};

const handleDelete = async (user) => {
  confirmService.require({
    message: `确定要删除用户 "${user.username}" 吗？此操作不可恢复。`,
    header: '删除确认',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '确定',
    rejectLabel: '取消',
    accept: async () => {
      try {
        await $fetch(`/api/admin/users/${user.id}`, {
          method: "DELETE",
        });
        
        toast.add({ severity: 'success', summary: '成功', detail: '删除成功', life: 3000 });
        refresh();
      } catch (error) {
        toast.add({ severity: 'error', summary: '错误', detail: '删除失败', life: 3000 });
        console.error(error);
      }
    }
  });
};

definePageMeta({ layout: "admin" });
</script>

<template>
  <Toast />
  <ConfirmDialog />

  <div class="admin-header flex align-items-center justify-content-between mb-4">
    <h1 class="text-2xl font-bold">用户管理</h1>
    <Button 
      label="创建用户" 
      icon="i-ri-add-line" 
      @click="dialogVisible = true"
    />
  </div>

  <DataTable :value="data ?? []" stripedRows>
    <Column field="id" header="ID" />
    <Column field="username" header="用户名" />
    <Column field="email" header="邮箱" />
    <Column field="phone" header="电话" />
    <Column field="admin" header="管理员">
      <template #body="slotProps">
        <Tag :severity="slotProps.data.admin ? 'success' : 'info'">
          {{ slotProps.data.admin ? '是' : '否' }}
        </Tag>
      </template>
    </Column>
    <Column header="操作">
      <template #body="slotProps">
        <Button icon="i-ri-pencil-line" text @click="editUser(slotProps.data)" />
        <Button icon="i-ri-delete-bin-line" text severity="danger" @click="handleDelete(slotProps.data)" />
      </template>
    </Column>
  </DataTable>

  <!-- 创建用户对话框 -->
  <Dialog 
    v-model:visible="dialogVisible" 
    header="创建用户" 
    modal 
    :style="{ width: '450px' }" 
    :closable="true"
  >
    <div class="p-fluid">
      <div class="field mb-4">
        <label for="username" class="block mb-1">用户名</label>
        <InputText 
          id="username" 
          v-model="form.username" 
          :class="{'p-invalid': submitted && !form.username}"
          aria-describedby="username-error"
        />
        <small v-if="submitted && !form.username" id="username-error" class="p-error">用户名不能为空</small>
      </div>

      <div class="field mb-4">
        <label for="password" class="block mb-1">密码</label>
        <Password 
          id="password" 
          v-model="form.password" 
          :class="{'p-invalid': submitted && !form.password}"
          :feedback="false"
          toggleMask
          aria-describedby="password-error"
        />
        <small v-if="submitted && !form.password" id="password-error" class="p-error">密码不能为空</small>
      </div>

      <div class="field mb-4">
        <label for="email" class="block mb-1">邮箱</label>
        <InputText id="email" v-model="form.email" type="email" />
      </div>

      <div class="field mb-4">
        <label for="phone" class="block mb-1">电话</label>
        <InputText id="phone" v-model="form.phone" />
      </div>

      <div class="field mb-4">
        <div class="flex align-items-center">
          <label for="admin" class="mr-2">管理员</label>
          <InputSwitch id="admin" v-model="form.admin" />
        </div>
      </div>
    </div>
    
    <template #footer>
      <Button label="取消" icon="i-ri-close-line" text @click="dialogVisible = false" />
      <Button label="创建" icon="i-ri-check-line" @click="submitForm" />
    </template>
  </Dialog>

  <!-- 编辑用户对话框 -->
  <Dialog 
    v-model:visible="editDialogVisible" 
    header="编辑用户" 
    modal 
    :style="{ width: '450px' }" 
    :closable="true"
  >
    <div class="p-fluid">
      <div class="field mb-4">
        <label for="edit-username" class="block mb-1">用户名</label>
        <InputText 
          id="edit-username" 
          v-model="form.username" 
          :class="{'p-invalid': submitted && !form.username}"
        />
        <small v-if="submitted && !form.username" class="p-error">用户名不能为空</small>
      </div>

      <div class="field mb-4">
        <label for="edit-password" class="block mb-1">密码</label>
        <Password 
          id="edit-password" 
          v-model="form.password" 
          placeholder="留空表示不修改密码"
          :feedback="false"
          toggleMask
        />
      </div>

      <div class="field mb-4">
        <label for="edit-email" class="block mb-1">邮箱</label>
        <InputText id="edit-email" v-model="form.email" type="email" />
      </div>

      <div class="field mb-4">
        <label for="edit-phone" class="block mb-1">电话</label>
        <InputText id="edit-phone" v-model="form.phone" />
      </div>

      <div class="field mb-4">
        <div class="flex align-items-center">
          <label for="edit-admin" class="mr-2">管理员</label>
          <InputSwitch id="edit-admin" v-model="form.admin" />
        </div>
      </div>
    </div>
    
    <template #footer>
      <Button label="取消" icon="i-ri-close-line" text @click="editDialogVisible = false" />
      <Button label="保存" icon="i-ri-check-line" @click="updateUser" />
    </template>
  </Dialog>
</template>

