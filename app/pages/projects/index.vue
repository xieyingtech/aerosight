<script setup lang="ts">
import type { NavigationMenuItem, TableColumn } from "@nuxt/ui";
import type { InternalApi } from "nitropack/types";

type ProjectScope = "all" | "joined" | "managed";
type ProjectListItem = InternalApi["/api/projects"]["get"][number];

const { t } = useI18n();
const route = useRoute();

const searchText = ref("");
const searchQuery = ref("");

const roleColors = {
  owner: "error",
  admin: "warning",
  member: "info",
} as const satisfies Record<ProjectRole, "error" | "warning" | "info">;

const activeScope = computed<ProjectScope>(() => {
  const scope = route.query.scope;
  if (scope === "joined" || scope === "managed") {
    return scope;
  }

  return "all";
});

const scopeItems = computed<NavigationMenuItem[]>(() => [
  {
    type: "label",
    label: t("projects.index.sidebar.title"),
  },
  {
    label: t("projects.index.scopes.all"),
    icon: "i-lucide-layers-3",
    to: "/projects",
    active: activeScope.value === "all",
  },
  {
    label: t("projects.index.scopes.joined"),
    icon: "i-lucide-users",
    to: "/projects?scope=joined",
    active: activeScope.value === "joined",
  },
  {
    label: t("projects.index.scopes.managed"),
    icon: "i-lucide-shield-check",
    to: "/projects?scope=managed",
    active: activeScope.value === "managed",
  },
]);

const columns: TableColumn<ProjectListItem>[] = [
  {
    header: t("projects.index.table.columns.name"),
    accessorKey: "name",
  },
  {
    header: t("projects.index.table.columns.team"),
    accessorKey: "teamName",
  },
  {
    header: t("projects.index.table.columns.role"),
    accessorKey: "role",
  },
  {
    header: t("projects.index.table.columns.updatedAt"),
    accessorKey: "updatedAt",
  },
];

const queryParams = computed(() => ({
  scope: activeScope.value === "all" ? undefined : activeScope.value,
  search: searchQuery.value || undefined,
}));

const {
  data: projectItems,
  pending,
  refresh,
} = await useFetch("/api/projects", {
  query: queryParams,
  default: () => [],
  watch: false,
});

watch(activeScope, () => {
  void refresh();
});

const resultCountLabel = computed(() =>
  t("projects.index.resultCount", { count: projectItems.value.length }),
);

function applySearch() {
  searchQuery.value = searchText.value.trim();
  void refresh();
}

function formatUpdatedAt(value: string | Date) {
  return new Date(value).toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    timeZone: "Asia/Shanghai",
  });
}
</script>

<template>
  <UPage>
    <template #left>
      <UPageAside>
        <UNavigationMenu :items="scopeItems" orientation="vertical" highlight />
      </UPageAside>
    </template>

    <UPageHeader
      :title="t('projects.index.title')"
      :description="resultCountLabel"
    />
    <UPageBody>
      <form class="w-full" @submit.prevent="applySearch">
        <UFieldGroup class="w-full">
          <UInput
            v-model="searchText"
            class="flex-1"
            :placeholder="t('projects.index.searchPlaceholder')"
          />
          <UButton
            type="submit"
            color="neutral"
            variant="subtle"
            icon="i-lucide-search"
            :aria-label="t('projects.index.searchButton')"
            :loading="pending"
          />
        </UFieldGroup>
      </form>

      <UTable :data="projectItems" :columns="columns" sticky class="flex-1">
        <template #name-cell="{ row }">
          <div class="space-y-1">
            <NuxtLink
              :to="`/projects/${row.original.id}`"
              class="font-medium text-highlighted hover:underline"
            >
              {{ row.original.name }}
            </NuxtLink>
            <p class="text-xs text-muted">
              {{
                row.original.description || t("projects.index.noDescription")
              }}
            </p>
          </div>
        </template>

        <template #teamName-cell="{ row }">
          <span class="text-sm text-muted">{{ row.original.teamName }}</span>
        </template>

        <template #role-cell="{ row }">
          <UBadge variant="subtle" :color="roleColors[row.original.role]">
            {{ t(`projects.index.role.${row.original.role}`) }}
          </UBadge>
        </template>

        <template #updatedAt-cell="{ row }">
          <span class="text-sm text-muted">
            {{ formatUpdatedAt(row.original.updatedAt) }}
          </span>
        </template>
      </UTable>
    </UPageBody>
  </UPage>
</template>
