<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";

const route = useRoute();

type DeviceItem = {
	id: number;
	name: string;
	type: string;
	status: string;
	lastSeenAt: string | null;
	updatedAt: string;
};

const projectId = computed(() => Number(route.params.id || 0));

const { data: items, pending } = await useFetch<DeviceItem[]>(
	() => `/api/projects/${projectId.value}/devices`,
	{ default: () => [] },
);

const columns: TableColumn<DeviceItem>[] = [
	{ accessorKey: "name", header: "名称" },
	{ accessorKey: "type", header: "类型" },
	{ accessorKey: "status", header: "状态" },
	{ accessorKey: "lastSeenAt", header: "最近在线" },
	{ accessorKey: "updatedAt", header: "更新于" },
];
</script>

<template>
	<UPageHeader title="设备监控" :description="`共 ${items.length} 台设备`" />

	<UPageBody>
		<UTable :data="items" :columns="columns" :loading="pending" sticky>
			<template #lastSeenAt-cell="{ row }">
				{{ row.original.lastSeenAt ? new Date(row.original.lastSeenAt).toLocaleString() : '-' }}
			</template>

			<template #updatedAt-cell="{ row }">
				{{ new Date(row.original.updatedAt).toLocaleDateString() }}
			</template>
		</UTable>
	</UPageBody>
</template>