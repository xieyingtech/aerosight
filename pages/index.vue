<script setup lang="ts">
const appConfig = useAppConfig();
const { user, loggedIn } = useUserSession();
const { data: memberships } = useFetch("/api/memberships");
const toast = useToast();
const confirm = useConfirm();

function navigateToDashboard() {
  const firstTeam = memberships.value?.[0]?.teamId;
  if (firstTeam) {
    navigateTo(`/teams/${firstTeam}`);
  } else {
    // 使用ConfirmDialog提示用户没有团队并询问是否创建
    confirm.require({
      message: '您目前没有任何团队，是否创建一个新团队？',
      header: '创建团队',
      icon: 'pi pi-exclamation-triangle',
      acceptLabel: '是',
      rejectLabel: '否',
      accept: () => {
        // 修改为跳转到teams/new.vue页面
        navigateTo('/teams/new');
      }
    });
  }
}
</script>

<template>
  <div class="flex gap-4">
    <NuxtLink v-if="loggedIn">
      <Button @click="navigateToDashboard">工作台</Button>
    </NuxtLink>
    <NuxtLink v-else to="/login">
      <Button>登录</Button>
    </NuxtLink>
    
     
  </div>
</template>
