<script setup lang="ts">
const { loggedIn, session, user, clear, fetch } = useUserSession();

definePageMeta({
  layout: false,
});

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
          ElMessage.error("账号或密码错误");
        });
    }
  });
}
</script>

<template>
  <ElCard class="max-w-sm mx-auto mt-40vh">
    <ElForm ref="formRef" label-width="auto" :model="form" :rules="rules">
      <ElFormItem label="用户名" prop="username">
        <ElInput v-model="form.username" />
      </ElFormItem>
      <ElFormItem label="密码" prop="password" required>
        <ElInput v-model="form.password" type="password" />
      </ElFormItem>
      <ElButton type="primary" @click="login">登录</ElButton>
    </ElForm>
  </ElCard>
</template>
