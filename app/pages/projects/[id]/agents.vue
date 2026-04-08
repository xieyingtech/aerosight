<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";

const route = useRoute();

type AgentItem = {
	id: number;
	name: string;
	description: string | null;
	status: string;
	updatedAt: string;
};

const projectId = computed(() => Number(route.params.id || 0));

const { data: items, pending } = await useFetch<AgentItem[]>(
	() => `/api/projects/${projectId.value}/agents`,
	{ default: () => [] },
);

const columns: TableColumn<AgentItem>[] = [
	{ accessorKey: "name", header: "名称" },
	{ accessorKey: "status", header: "状态" },
	{ accessorKey: "description", header: "说明" },
	{ accessorKey: "updatedAt", header: "更新于" },
];
</script>

<template>
	<UPageHeader title="智能体" :description="`共 ${items.length} 个智能体`" />

	<UPageBody>
		<UTable :data="items" :columns="columns" :loading="pending" sticky>
			<template #updatedAt-cell="{ row }">
				{{ new Date(row.original.updatedAt).toLocaleDateString() }}
			</template>
		</UTable>
	</UPageBody>
</template>