<script setup lang="ts">
import type { AuthFormField, FormSubmitEvent } from "@nuxt/ui";
import { z } from "zod";

const { site } = useAppConfig();
const { t } = useI18n();
const toast = useToast();

const fields = computed<AuthFormField[]>(() => [
  {
    name: "username",
    label: t("auth.login.fields.username.label"),
    type: "text",
    placeholder: t("auth.login.fields.username.placeholder"),
  },
  {
    name: "password",
    label: t("auth.login.fields.password.label"),
    type: "password",
    placeholder: t("auth.login.fields.password.placeholder"),
  },
]);

const schema = z.object({
  username: z.string().min(1, t("errors.validation.username.required")),
  password: z.string().min(1, t("errors.validation.password.required")),
});

const loginTitle = computed(() => t("auth.login.title", { site: site.title }));

function resolveMessage(message?: string) {
  if (!message) {
    return t("errors.generic");
  }

  return message.startsWith("errors.") ? t(message) : message;
}

function login(payload: FormSubmitEvent<z.infer<typeof schema>>) {
  const account = payload.data.username.trim();
  const requestBody = {
    password: payload.data.password,
    ...(account.includes("@") ? { email: account } : { phone: account }),
  };

  $fetch("/api/auth/login", {
    method: "POST",
    body: requestBody,
  })
    .then(() => {
      navigateTo("/console");
    })
    .catch((err) => {
      toast.add({
        title: t("auth.login.failed"),
        description: resolveMessage(err.data?.message || err.message),
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
        :title="loginTitle"
        :fields="fields"
        :schema="schema"
        @submit="login"
      />
    </UPageBody>
  </UPage>
</template>
