<script setup lang="ts">
import type { NavigationMenuItem, TableColumn } from "@nuxt/ui";
import type { InternalApi } from "nitropack/types";

type ProfileTab = "info" | "teams";
type ProfileItem = InternalApi["/api/profile"]["get"][number];
type TeamItem = InternalApi["/api/profile/teams"]["get"][number];

const { t } = useI18n();
const route = useRoute();
const { user } = useUserSession();
const toast = useToast();
const fetchApi = $fetch;

if (!user.value) {
  navigateTo("/login");
}

const roleColors = {
  owner: "error",
  admin: "warning",
  member: "info",
} as const satisfies Record<TeamItem["role"], "error" | "warning" | "info">;

const activeTab = computed<ProfileTab>(() => {
  return route.query.tab === "teams" ? "teams" : "info";
});

const menuItems = computed<NavigationMenuItem[]>(() => [
  {
    type: "label",
    label: t("profile.sidebar.title"),
  },
  {
    label: t("profile.tabs.info"),
    icon: "i-lucide-user-round",
    to: "/profile",
    active: activeTab.value === "info",
  },
  {
    label: t("profile.tabs.teams"),
    icon: "i-lucide-users",
    to: "/profile?tab=teams",
    active: activeTab.value === "teams",
  },
]);

const teamColumns: TableColumn<TeamItem>[] = [
  {
    header: t("profile.teams.table.columns.name"),
    accessorKey: "name",
  },
  {
    header: t("profile.teams.table.columns.role"),
    accessorKey: "role",
  },
  {
    header: t("profile.teams.table.columns.joinedAt"),
    accessorKey: "joinedAt",
  },
];

const {
  data: profileItems,
  pending: profilePending,
  refresh: refreshProfile,
} = await useFetch("/api/profile", {
  default: () => [],
  watch: false,
});

const { data: teamItems, pending: teamPending } = await useFetch(
  "/api/profile/teams",
  {
    default: () => [],
    watch: false,
  },
);

const profile = computed(
  () => profileItems.value[0] as ProfileItem | undefined,
);

const formState = reactive({
  name: "",
  email: "",
  phone: "",
});

watch(
  profile,
  (value) => {
    if (!value) {
      formState.name = "";
      formState.email = "";
      formState.phone = "";
      return;
    }

    formState.name = value.name || "";
    formState.email = value.email || "";
    formState.phone = value.phone || "";
  },
  { immediate: true },
);

const saving = ref(false);

function resolveMessage(message?: string) {
  if (!message) {
    return t("errors.generic");
  }

  return message.startsWith("errors.") ? t(message) : message;
}

async function saveProfile() {
  saving.value = true;

  try {
    await fetchApi("/api/profile", {
      method: "PATCH",
      body: {
        name: formState.name,
        email: formState.email,
        phone: formState.phone,
      },
    });

    await refreshProfile();
    toast.add({
      title: t("profile.info.saveSuccess"),
      color: "success",
    });
  } catch (e) {
    const error = e as { data?: { message?: string }; message?: string };
    toast.add({
      title: t("profile.info.saveFailed"),
      description: resolveMessage(error.data?.message || error.message),
      color: "error",
    });
  } finally {
    saving.value = false;
  }
}

const pageDescription = computed(() => {
  if (activeTab.value === "teams") {
    return t("profile.teams.resultCount", { count: teamItems.value.length });
  }

  return t("profile.info.description");
});
</script>

<template>
  <UPage>
    <template #left>
      <UPageAside>
        <UNavigationMenu :items="menuItems" orientation="vertical" highlight />
      </UPageAside>
    </template>

    <UPageHeader :title="t('profile.title')" :description="pageDescription" />

    <UPageBody>
      <UForm
        v-if="activeTab === 'info'"
        :state="formState"
        class="max-w-xl space-y-4"
        @submit="saveProfile"
      >
        <UFormField :label="t('profile.info.fields.id')">
          <UInput
            class="w-full"
            :model-value="String(profile?.id || '')"
            disabled
          />
        </UFormField>

        <UFormField :label="t('profile.info.fields.role')">
          <UInput
            class="w-full"
            :model-value="
              profile ? t(`projects.index.role.${profile.role}`) : ''
            "
            :loading="profilePending"
            disabled
          />
        </UFormField>

        <UFormField name="name" :label="t('profile.info.fields.name')" required>
          <UInput
            class="w-full"
            v-model="formState.name"
            :placeholder="t('profile.info.placeholders.name')"
            :loading="profilePending"
          />
        </UFormField>

        <UFormField name="email" :label="t('profile.info.fields.email')">
          <UInput
            class="w-full"
            v-model="formState.email"
            :placeholder="t('profile.info.placeholders.email')"
            :loading="profilePending"
          />
        </UFormField>

        <UFormField name="phone" :label="t('profile.info.fields.phone')">
          <UInput
            class="w-full"
            v-model="formState.phone"
            :placeholder="t('profile.info.placeholders.phone')"
            :loading="profilePending"
          />
        </UFormField>

        <UFormField :label="t('profile.info.fields.createdAt')">
          <UInput
            class="w-full"
            :model-value="
              profile ? new Date(profile.createdAt).toLocaleDateString() : ''
            "
            :loading="profilePending"
            disabled
          />
        </UFormField>

        <div class="pt-2">
          <UButton
            type="submit"
            icon="i-lucide-save"
            :label="t('profile.info.save')"
            :loading="saving"
          />
        </div>
      </UForm>

      <UTable
        v-else
        :data="teamItems"
        :columns="teamColumns"
        :loading="teamPending"
        sticky
      >
        <template #role-cell="{ row }">
          <UBadge variant="subtle" :color="roleColors[row.original.role]">
            {{ t(`projects.index.role.${row.original.role}`) }}
          </UBadge>
        </template>

        <template #joinedAt-cell="{ row }">
          <span class="text-sm text-muted">
            {{ new Date(row.original.joinedAt).toLocaleDateString() }}
          </span>
        </template>
      </UTable>
    </UPageBody>
  </UPage>
</template>
