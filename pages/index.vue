<script setup lang="ts">
const appConfig = useAppConfig();
const { loggedIn } = useUserSession();
const { data: memberships, pending } = await useFetch("/api/memberships", {
  watch: [loggedIn],
});
</script>

<template>
  <div class="w-full">
    <div v-if="loggedIn" class="bg-gray-100 p-6 rounded-lg">
      <ElCard
        v-for="membership in memberships"
        class="mb-4 cursor-pointer hover:shadow-md transition-shadow"
        @click="navigateTo(`/orgs/${membership.organizationId}`)"
      >
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="font-bold">{{ membership.organization.name }}</h3>
            <ElButton
              type="primary"
              text
              @click.stop="navigateTo(`/orgs/${membership.organizationId}`)"
            >
              查看
            </ElButton>
          </div>
        </template>
        <div>
          <p class="text-gray-600 mb-3">{{ membership.name }}</p>
          <div class="flex flex-wrap gap-2">
            <ElTag
              v-for="(role, idx) in membership.roles"
              :key="idx"
              size="small"
              type="info"
            >
              {{ role }}
            </ElTag>
          </div>
        </div>
      </ElCard>
    </div>
    <div v-else>
      <ElResult title="请先登录" sub-title="您需要登录才能查看组织信息">
        <template #extra>
          <ElButton type="primary" @click="navigateTo('/login')">登录</ElButton>
        </template>
      </ElResult>
    </div>
  </div>
</template>
