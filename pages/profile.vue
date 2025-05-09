<script setup lang="ts">
const { user, clear } = useUserSession();
const { data: memberships } = useFetch("/api/memberships");

function logout() {
  clear();
  navigateTo("/login");
}
</script>

<template>
  <div class="p-4">
    <h1 class="text-2xl font-bold mb-6">个人资料</h1>
    
    <Card>
      <template #header>
        <div class="flex justify-between items-center">
          <h2 class="text-xl">用户信息</h2>
        </div>
      </template>
      <template #content>
        <div class="grid">
          <div class="col-12 field">
            <label class="font-bold mb-2">用户名</label>
            <div>{{ user.username }}</div>
          </div>
          <div class="col-12 field">
            <label class="font-bold mb-2">角色</label>
            <div>{{ user.admin ? '管理员' : '普通用户' }}</div>
          </div>
        </div>
      </template>
    </Card>
    
    <div class="mt-6">
      <h2 class="text-xl font-bold mb-4">我的团队</h2>
      <div v-if="memberships && memberships.length > 0" class="grid">
        <div v-for="membership in memberships" :key="membership.id" class="col-12 mb-3">
          <Card class="shadow-1">
            <template #content>
              <div class="flex justify-between">
                <h3 class="font-bold">{{ membership.team.name }}</h3>
                <Button 
                  label="进入团队"
                  size="small" 
                  @click="navigateTo(`/teams/${membership.teamId}`)"
                />
              </div>
              <div class="mt-2">
                <p>{{ membership.name }}</p>
                <div class="mt-2 flex flex-wrap gap-1">
                  <Tag 
                    v-for="(role, index) in membership.roles" 
                    :key="index"
                    severity="info"
                  >
                    {{ role }}
                  </Tag>
                </div>
              </div>
            </template>
          </Card>
        </div>
      </div>
      <div v-else class="empty-container flex align-items-center justify-content-center p-5 flex-column">
        <i class="i-ri-team-line text-4xl text-gray-400 mb-3"></i>
        <span class="text-gray-400">您暂未加入任何团队</span>
      </div>
    </div>
    
    <div class="mt-6 text-right">
      <Button severity="danger" label="退出登录" icon="i-ri-logout-box-line" @click="logout" />
    </div>
  </div>
</template>
