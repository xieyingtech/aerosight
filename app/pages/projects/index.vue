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

function search() {
  searchQuery.value = searchText.value.trim();
  void refresh();
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
      <form class="w-full" @submit.prevent="search">
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
            <p class="flex items-center gap-2">
              <ULink
                :to="`/projects/${row.original.id}`"
                class="min-w-0 text-sm hover:underline"
              >
                <span class="text-dimmed">{{ row.original.teamName }}/</span>
                <span class="font-medium text-highlighted">
                  {{ row.original.name }}
                </span>
              </ULink>
              <UBadge
                variant="subtle"
                :color="roleColors[row.original.role]"
                class="shrink-0"
              >
                {{ t(`projects.index.role.${row.original.role}`) }}
              </UBadge>
            </p>
            <p class="text-xs text-muted">
              {{
                row.original.description || t("projects.index.noDescription")
              }}
            </p>
          </div>
        </template>

        <template #updatedAt-cell="{ row }">
          <span class="text-sm text-muted">
            {{ new Date(row.original.updatedAt).toLocaleDateString() }}
          </span>
        </template>
      </UTable>
    </UPageBody>
  </UPage>
</template>
