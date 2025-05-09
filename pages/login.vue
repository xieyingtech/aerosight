<script setup lang="ts">
import type { FormSubmitEvent } from "@primevue/forms";
import { zodResolver } from "@primevue/forms/resolvers/zod";
import { z } from "zod";

const { fetch } = useUserSession();
const appConfig = useAppConfig();
const toast = useToast();

const resolver = zodResolver(
  z.object({
    username: z.string().nonempty("请输入用户名"),
    password: z.string().nonempty("请输入密码"),
  })
);

const onFormSubmit = async (e: FormSubmitEvent) => {
  if (!e.valid) return;
  try {
    await $fetch("/api/login", {
      method: "POST",
      body: e.values,
    });
    toast.add({
      severity: "success",
      summary: "成功",
      detail: "登录成功",
      life: 3000,
    });
    await fetch();
    navigateTo("/");
  } catch (e: any) {
    if (e.response?.status === 401) {
      toast.add({
        severity: "error",
        summary: "错误",
        detail: "用户名或密码错误",
      });
    } else {
      toast.add({
        severity: "error",
        summary: "错误",
        detail: e.message,
        life: 3000,
      });
    }
  }
};
</script>

<template>
  <div class="w-full max-w-sm mx-auto p-4 box-border">
    <h1 class="text-center">登录到 {{ appConfig.site.title }}</h1>
    <Form
      v-slot="$form"
      :resolver="resolver"
      @submit="onFormSubmit"
      class="flex flex-col gap-4"
    >
      <div class="flex flex-col gap-1">
        <InputText name="username" type="text" placeholder="用户名" fluid />
        <Message
          v-if="$form.username?.invalid"
          severity="error"
          size="small"
          variant="simple"
        >
          {{ $form.username.error.message }}
        </Message>
      </div>
      <div class="flex flex-col gap-1">
        <Password
          name="password"
          placeholder="密码"
          :feedback="false"
          toggleMask
          fluid
        />
        <Message
          v-if="$form.password?.invalid"
          severity="error"
          size="small"
          variant="simple"
        >
          {{ $form.password.error.message }}
        </Message>
      </div>
      <Button type="submit" label="登录" />
    </Form>
  </div>
</template>
