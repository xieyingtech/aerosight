<script setup lang="ts">
import * as locales from "@nuxt/ui/locale";

const { locale } = useI18n();
const { site } = useAppConfig();

useHead({
  titleTemplate: (title?: string) =>
    title ? `${title} - ${site.title}` : site.title,
});
</script>

<template>
  <UApp :locale="locales[locale]">
    <NuxtLayout>
      <NuxtPage />
    </NuxtLayout>
  </UApp>
</template>

<style>
@import "tailwindcss";
@import "@nuxt/ui";

@theme {
  --color-primary: #3b82f6;
}
</style>
