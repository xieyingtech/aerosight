<script setup lang="ts">
const { loggedIn, session, user, clear, fetch } = useUserSession()
const toast = useToast()

const state = reactive({
  username: '',
  password: ''
})

async function login() {
  $fetch('/api/login', {
    method: 'POST',
    body: state
  })
    .then(async () => {
      await fetch()
    })
    .catch((e) => { toast.add({ title: e }) })
}
</script>

<template>
  <div v-if="loggedIn">
    <p>用户名：{{ user }}</p>
  </div>
  <UContainer class="max-w-sm mt-32">
    <UCard>
      <UForm class="space-y-4" :state @submit="login">
        <UFormGroup label="用户名" required>
          <UInput v-model="state.username" />
        </UFormGroup>
        <UFormGroup label="密码" required>
          <UInput v-model="state.password" type="password" />
        </UFormGroup>
        <UButton block type="submit">登录</UButton>
      </UForm>
    </UCard>
  </UContainer>
</template>