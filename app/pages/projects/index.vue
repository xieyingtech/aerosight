<script setup lang="ts">
import type { NavigationMenuItem, TableColumn } from "@nuxt/ui";

type ProjectScope = "all" | "joined" | "managed";
type Role = "owner" | "admin" | "member";

interface ProjectItem {
  id: number;
  name: string;
  description: string | null;
  teamName: string;
  role: Role;
  updatedAt: string;
}

const { t } = useI18n();
const route = useRoute();

const searchState = reactive({ search: "" });
const searchQuery = ref("");

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

const columns: TableColumn<ProjectItem>[] = [
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

const { data: projectItems, pending } = await useFetch<ProjectItem[]>(
  "/api/projects",
  {
    query: queryParams,
    default: () => [],
  },
);

const resultCountLabel = computed(() =>
  t("projects.index.resultCount", { count: projectItems.value.length }),
);

function applySearch() {
  searchQuery.value = searchState.search.trim();
}

function roleColor(role: Role) {
  if (role === "owner") {
    return "error";
  }

  if (role === "admin") {
    return "warning";
  }

  return "info";
}

function roleText(role: Role) {
  return t(`projects.index.role.${role}`);
}

function formatUpdatedAt(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date(value));
}
</script>

<template>
  <UPage>
    <template #left>
      <UPageAside>
        <UNavigationMenu :items="scopeItems" orientation="vertical" highlight />
      </UPageAside>
    </template>

    <UPageBody>
      <div class="space-y-4">
        <UPageHeader
          :title="t('projects.index.title')"
          :description="resultCountLabel"
        />

        <UForm class="w-full md:w-[28rem]" :state="searchState" @submit="applySearch">
          <UFieldGroup class="w-full">
            <UInput
              v-model="searchState.search"
              icon="i-lucide-search"
              :placeholder="t('projects.index.searchPlaceholder')"
            />
            <UButton
              type="submit"
              color="neutral"
              variant="subtle"
              icon="i-lucide-search"
              :label="t('projects.index.searchButton')"
              :loading="pending"
            />
          </UFieldGroup>
        </UForm>

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
                {{ row.original.description || t("projects.index.noDescription") }}
              </p>
            </div>
          </template>

          <template #teamName-cell="{ row }">
            <span class="text-sm text-muted">{{ row.original.teamName }}</span>
          </template>

          <template #role-cell="{ row }">
            <UBadge variant="subtle" :color="roleColor(row.original.role)">
              {{ roleText(row.original.role) }}
            </UBadge>
          </template>

          <template #updatedAt-cell="{ row }">
            <span class="text-sm text-muted">
              {{ formatUpdatedAt(row.original.updatedAt) }}
            </span>
          </template>
        </UTable>
      </div>
    </UPageBody>
  </UPage>
</template>
