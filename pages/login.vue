<script setup lang="ts">
const { loggedIn, session, user, clear, fetch } = useUserSession();

const appConfig = useAppConfig();

const formRef = useTemplateRef("formRef");

const form = reactive({
  username: "",
  password: "",
});

const rules = {
  username: [{ required: true, message: "请输入用户名", trigger: "blur" }],
  password: [{ required: true, message: "请输入密码", trigger: "blur" }],
};

async function login() {
  formRef.value?.validate((valid, fields) => {
    if (valid) {
      $fetch("/api/login", {
        method: "POST",
        body: form,
      })
        .then(async () => {
          ElMessage.success("登录成功");
          await fetch();
          navigateTo("/");
        })
        .catch((e) => {
          if (e.response.status === 401) {
            ElMessage.error("用户名或密码错误");
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
    <h1 class="text-center">登录到 {{ appConfig.site.title }}</h1>
    <ElForm ref="formRef" label-width="auto" :model="form" :rules="rules">
      <ElFormItem label="账号" prop="username">
        <ElInput v-model="form.username" />
      </ElFormItem>
      <ElFormItem label="密码" prop="password" required>
        <ElInput v-model="form.password" type="password" />
      </ElFormItem>
      <ElButton type="primary" class="w-full" @click="login">登录</ElButton>
    </ElForm>
  </div>
</template>
