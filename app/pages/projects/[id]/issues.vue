<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";

const route = useRoute();

type IssueItem = {
	id: number;
	number: number;
	title: string;
	status: string;
	priority: string;
	updatedAt: string;
};

const projectId = computed(() => Number(route.params.id || 0));

const { data: items, pending } = await useFetch<IssueItem[]>(
	() => `/api/projects/${projectId.value}/issues`,
	{ default: () => [] },
);

const columns: TableColumn<IssueItem>[] = [
	{ accessorKey: "number", header: "编号" },
	{ accessorKey: "title", header: "标题" },
	{ accessorKey: "status", header: "状态" },
	{ accessorKey: "priority", header: "优先级" },
	{ accessorKey: "updatedAt", header: "更新于" },
];
</script>

<template>
	<UPageHeader title="问题中心" :description="`共 ${items.length} 个问题`" />

	<UPageBody>
		<UTable :data="items" :columns="columns" :loading="pending" sticky>
			<template #number-cell="{ row }">
				#{{ row.original.number }}
			</template>

			<template #updatedAt-cell="{ row }">
				{{ new Date(row.original.updatedAt).toLocaleDateString() }}
			</template>
		</UTable>
	</UPageBody>
</template>