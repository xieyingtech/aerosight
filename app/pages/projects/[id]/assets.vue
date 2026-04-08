<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";

const route = useRoute();

type AssetItem = {
	id: number;
	kind: string;
	mimeType: string | null;
	capturedAt: string | null;
	createdAt: string;
};

const projectId = computed(() => Number(route.params.id || 0));

const { data: items, pending } = await useFetch<AssetItem[]>(
	() => `/api/projects/${projectId.value}/assets`,
	{ default: () => [] },
);

const columns: TableColumn<AssetItem>[] = [
	{ accessorKey: "kind", header: "类型" },
	{ accessorKey: "mimeType", header: "MIME" },
	{ accessorKey: "capturedAt", header: "采集时间" },
	{ accessorKey: "createdAt", header: "创建于" },
];
</script>

<template>
  <UPageHeader title="素材库" :description="`共 ${items.length} 条素材`" />

  <UPageBody>
    <UTable :data="items" :columns="columns" :loading="pending" sticky>
      <template #capturedAt-cell="{ row }">
        {{ row.original.capturedAt ? new Date(row.original.capturedAt).toLocaleString() : '-' }}
      </template>

      <template #createdAt-cell="{ row }">
        {{ new Date(row.original.createdAt).toLocaleDateString() }}
      </template>
    </UTable>
  </UPageBody>
</template>