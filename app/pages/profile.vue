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
    <UCard>
      <template #header>
        <span class="font-bold">昵称</span>
      </template>
      <div class="flex gap-2">
  <UInput v-model="profile.name" />
  <UButton @click="updateUser('name')" icon="i-ri-save-line">保存</UButton>
      </div>
    </UCard>

    <UCard>
      <template #header>
        <span class="font-bold">用户名</span>
      </template>
      <div class="flex gap-2">
  <UInput v-model="profile.username" />
  <UButton @click="updateUser('username')" icon="i-ri-save-line">保存</UButton>
      </div>
    </UCard>

    <div class="text-right">
      <UButton
        color="red"
        label="退出登录"
        icon="i-ri-logout-box-line"
        @click="logout"
      />
    </div>
  </div>
</template>
