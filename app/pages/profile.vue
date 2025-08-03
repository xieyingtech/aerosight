<script setup lang="ts">
const { clear } = useUserSession();
const { data: profile } = useFetch("/api/user");
const { data: memberships } = useFetch("/api/memberships");
const toast = useToast();

function logout() {
  clear();
  navigateTo("/login");
}

// 修改现有的 updateUser 函数
async function updateUser(field) {
  try {
    if (!profile.value) return;
    
    const updateData = {};
    if (field === 'name') {
      updateData.name = profile.value.name;
    } else if (field === 'username') {
      updateData.username = profile.value.username;
    }
    
    await $fetch("/api/user", {
      method: "PUT",
      body: updateData
    });
    
    toast.add({
      severity: "success",
      summary: "成功",
      detail: "用户信息已更新",
      life: 3000,
    });
  } catch (error) {
    toast.add({
      severity: "error",
      summary: "错误",
      detail: "更新用户信息失败",
      life: 3000,
    });
    console.error(error);
  }
}

</script>

<template>
  <div v-if="profile" class="flex flex-col gap-4">
    <Panel header="昵称">
      <div class="inline-flex gap-2">
        <InputText v-model="profile.name" />
        <Button @click="updateUser('name')">保存</Button>
      </div>
    </Panel>

    <Panel header="用户名">
      <div class="inline-flex gap-2">
        <InputText v-model="profile.username" />
        <Button @click="updateUser('username')">保存</Button>
      </div>
    </Panel>

    <div class="text-right">
      <Button
        severity="danger"
        label="退出登录"
        icon="i-ri-logout-box-line"
        @click="logout"
      />
    </div>
  </div>
</template>
