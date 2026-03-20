<script setup lang="ts">
import type { AuthFormField, FormSubmitEvent } from "@nuxt/ui";
import { z } from "zod";

const { site } = useAppConfig();
const toast = useToast();

const fields: AuthFormField[] = [
  {
    name: "username",
    label: "用户名",
    type: "text",
    placeholder: "请输入用户名",
  },
  {
    name: "password",
    label: "密码",
    type: "password",
    placeholder: "请输入密码",
  },
];

const schema = z.object({
  username: z.string("用户名不能为空"),
  password: z.string("密码不能为空"),
});

function login(payload: FormSubmitEvent<z.infer<typeof schema>>) {
  $fetch("/api/auth/login", {
    method: "POST",
    body: payload,
  })
    .then(() => {
      navigateTo("/console");
    })
    .catch((err) => {
      toast.add({
        title: "登录失败",
        description: err.data.message || err.message,
        color: "error",
      });
    });
}
</script>

<template>
  <UPage>
    <UPageBody>
      <UAuthForm
        class="w-md mx-auto"
        :title="`登录到 ${site.title}`"
        :fields="fields"
        :schema="schema"
        @submit="login"
      />
    </UPageBody>
  </UPage>
</template>
