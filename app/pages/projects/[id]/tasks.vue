<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";

const route = useRoute();

type TaskItem = {
	id: number;
	name: string;
	description: string | null;
	triggerType: string;
	status: string;
	updatedAt: string;
};

const projectId = computed(() => Number(route.params.id || 0));

const { data: items, pending } = await useFetch<TaskItem[]>(
	() => `/api/projects/${projectId.value}/tasks`,
	{ default: () => [] },
);

const columns: TableColumn<TaskItem>[] = [
	{ accessorKey: "name", header: "名称" },
	{ accessorKey: "triggerType", header: "触发方式" },
	{ accessorKey: "status", header: "状态" },
	{ accessorKey: "updatedAt", header: "更新于" },
];
</script>

<template>
	<UPageHeader title="任务编排" :description="`共 ${items.length} 个任务`" />

	<UPageBody>
		<UTable :data="items" :columns="columns" :loading="pending" sticky>
			<template #updatedAt-cell="{ row }">
				{{ new Date(row.original.updatedAt).toLocaleDateString() }}
			</template>
		</UTable>
	</UPageBody>
</template>