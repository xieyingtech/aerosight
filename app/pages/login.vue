<script setup lang="ts">
import { z } from "zod";

const { fetch } = useUserSession();
const appConfig = useAppConfig();

const schema = z.object({
  username: z.string().min(1, "请输入用户名"),
  password: z.string().min(1, "请输入密码"),
});

type Schema = z.output<typeof schema>;

const state = reactive({
  username: "",
  password: "",
});

const onSubmit = async (event: any) => {
  try {
    await $fetch("/api/login", {
      method: "POST",
      body: event.data,
    });
    await fetch();
    navigateTo("/");
  } catch (e: any) {
    // 暂时简化错误处理
    console.error(e);
  }
};
</script>

<template>
  <div class="w-full max-w-sm mx-auto p-4">
    <h1 class="text-center text-2xl font-bold mb-6">登录到 {{ appConfig.site.title }}</h1>
    <UForm
      :schema="schema"
      :state="state"
      @submit="onSubmit"
      class="space-y-4"
    >
      <UFormGroup label="用户名" name="username">
        <UInput v-model="state.username" placeholder="请输入用户名" />
      </UFormGroup>
      
      <UFormGroup label="密码" name="password">
        <UInput v-model="state.password" type="password" placeholder="请输入密码" />
      </UFormGroup>
      
      <UButton type="submit" block>
        登录
      </UButton>
    </UForm>
  </div>
</template>
