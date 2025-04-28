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
    
    <ElCard>
      <template #header>
        <div class="flex justify-between items-center">
          <h2 class="text-xl">用户信息</h2>
        </div>
      </template>
      <ElDescriptions :column="1" border>
        <ElDescriptionsItem label="用户名">{{ user.username }}</ElDescriptionsItem>
        <ElDescriptionsItem label="角色">
          {{ user.admin ? '管理员' : '普通用户' }}
        </ElDescriptionsItem>
      </ElDescriptions>
    </ElCard>
    
    <div class="mt-6">
      <h2 class="text-xl font-bold mb-4">我的团队</h2>
      <div v-if="memberships && memberships.length > 0" class="space-y-4">
        <ElCard v-for="membership in memberships" :key="membership.id" shadow="hover">
          <div class="flex justify-between">
            <h3 class="font-bold">{{ membership.team.name }}</h3>
            <ElButton 
              type="primary" 
              size="small" 
              @click="navigateTo(`/teams/${membership.teamId}`)"
            >
              进入团队
            </ElButton>
          </div>
          <div class="mt-2">
            <p>{{ membership.name }}</p>
            <div class="mt-2">
              <ElTag 
                v-for="(role, index) in membership.roles" 
                :key="index"
                size="small" 
                class="mr-1"
              >
                {{ role }}
              </ElTag>
            </div>
          </div>
        </ElCard>
      </div>
      <ElEmpty v-else description="您暂未加入任何团队" />
    </div>
    
    <div class="mt-6 text-right">
      <ElButton @click="logout" type="danger">退出登录</ElButton>
    </div>
  </div>
</template>
