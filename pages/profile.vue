<script setup lang="ts">
const { clear } = useUserSession();
const { data: profile } = useFetch("/api/user");
const { data: memberships } = useFetch("/api/memberships");

function logout() {
  clear();
  navigateTo("/login");
}
</script>

<template>
  <div v-if="profile" class="flex flex-col gap-4">
    <Panel header="昵称">
      <div class="inline-flex gap-2">
        <InputText v-model="profile.name" />
        <Button>保存</Button>
      </div>
    </Panel>

    <Panel header="用户名">
      <div class="inline-flex gap-2">
        <InputText v-model="profile.username" />
        <Button>保存</Button>
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
