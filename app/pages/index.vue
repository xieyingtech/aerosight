<script setup lang="ts">
const { site } = useAppConfig();
</script>

<template>
  <UPage>
    <UPageBody>
      <UPageCTA
        :title="site.title"
        :description="site.description"
        orientation="horizontal"
        variant="naked"
      >
        <template #links>
          <UButton>立即开始</UButton>
        </template>
        <UIcon name="i-lucide-drone" class="not-lg:hidden w-full text-primary size-48" />
      </UPageCTA>
    </UPageBody>
  </UPage>
</template>
