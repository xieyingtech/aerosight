<script setup lang="ts">
const appConfig = useAppConfig();
const { user, loggedIn } = useUserSession();
const { data: memberships } = useFetch("/api/memberships");
const toast = useToast();

function navigateToDashboard() {
  const firstTeam = memberships.value?.[0]?.teamId;
  if (firstTeam) {
    navigateTo(`/teams/${firstTeam}`);
  } else {
    toast.add({
      severity: "warn",
      summary: "没有团队",
      detail: "请先创建一个团队",
    });
  }
}
</script>

<template>
  <NuxtLink v-if="loggedIn">
    <Button @click="navigateToDashboard">工作台</Button>
  </NuxtLink>
  <NuxtLink v-else to="/login">
    <Button>登录</Button>
  </NuxtLink>
</template>
