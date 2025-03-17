<script setup lang="ts">
import { e } from "unocss";

const { fetch } = useUserSession();

const appConfig = useAppConfig();

const formRef = useTemplateRef("formRef");

const form = reactive({
  email: "",
  username: "",
  password: "",
});

const rules = {
  email: [
    { required: true, message: "请输入邮箱", trigger: "blur" },
    {
      validator: (_: any, value: string, callback: any) => {
        if (!/^[a-zA-Z0-9_-]+@[a-zA-Z0-9_-]+(\.[a-zA-Z0-9_-]+)+$/.test(value)) {
          callback(new Error("请输入正确的邮箱"));
        } else {
          callback();
        }
      },
      message: "请输入正确的邮箱",
      trigger: "blur",
    },
  ],
  username: [{ required: true, message: "请输入管理员账号", trigger: "blur" }],
  password: [{ required: true, message: "请输入管理员密码", trigger: "blur" }],
};

async function login() {
  formRef.value?.validate((valid, fields) => {
    if (valid) {
      $fetch("/api/admin/init", {
        method: "POST",
        body: form,
      })
        .then(async () => {
          ElMessage.success("初始化成功");
          await fetch();
          navigateTo("/login");
        })
        .catch((e) => {
          if (e.response.status === 400) {
            ElMessage.error("已初始化管理员账号");
            navigateTo("/login");
          } else {
            ElMessage.error(e.message);
          }
        });
    }
  });
}
</script>

<template>
  <div class="w-full max-w-sm mx-auto px-4">
    <h1 class="text-center">初始化 {{ appConfig.site.name }}</h1>
    <ElForm ref="formRef" label-width="auto" :model="form" :rules="rules">
      <ElFormItem label="管理员邮箱" prop="email">
        <ElInput v-model="form.email" />
      </ElFormItem>
      <ElFormItem label="管理员账号" prop="username">
        <ElInput v-model="form.username" />
      </ElFormItem>
      <ElFormItem label="管理员密码" prop="password" required>
        <ElInput v-model="form.password" type="password" />
      </ElFormItem>
      <ElButton type="primary" class="w-full" @click="login">初始化</ElButton>
    </ElForm>
  </div>
</template>
