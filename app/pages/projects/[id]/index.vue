<script setup lang="ts">
const route = useRoute();

type ProjectDetail = {
	id: number;
	name: string;
	description: string | null;
	teamId: number;
	teamName: string;
	role: ProjectRole;
	updatedAt: string;
};

const projectId = computed(() => Number(route.params.id || 0));

const { data: project, pending } = await useAsyncData<ProjectDetail | null>(
	() => `project-${projectId.value}`,
	() => $fetch<ProjectDetail>(`/api/projects/${projectId.value}`),
	{
		default: () => null,
	},
);
</script>

<template>
  <UPageHeader
    :title="project?.name || '项目概览'"
    :description="
      project
        ? `${project.teamName} · ${project.role}`
        : `当前项目 ID：${projectId}`
    "
  />

  <UPageBody>
    <UPageGrid>
      <UCard>
        <template #header>
          <div class="text-sm text-muted">所属团队</div>
        </template>
        <div class="text-lg font-medium">
          {{ pending ? '-' : project?.teamName || '-' }}
        </div>
      </UCard>

      <UCard>
        <template #header>
          <div class="text-sm text-muted">我的角色</div>
        </template>
        <div class="text-lg font-medium">
          {{ pending ? '-' : project?.role || '-' }}
        </div>
      </UCard>

      <UCard>
        <template #header>
          <div class="text-sm text-muted">最近更新</div>
        </template>
        <div class="text-lg font-medium">
          {{
            pending || !project
              ? '-'
              : new Date(project.updatedAt).toLocaleDateString()
          }}
        </div>
      </UCard>
    </UPageGrid>

    <UCard v-if="project?.description" class="mt-6">
      <template #header>
        <div class="text-sm text-muted">项目描述</div>
      </template>
      <p class="text-sm text-muted">
        {{ project.description }}
      </p>
    </UCard>
  </UPageBody>
</template>