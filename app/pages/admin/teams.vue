<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";

const { t } = useI18n();
const toast = useToast();
const fetchApi = $fetch;

const {
  data: teams,
  pending,
  refresh,
} = await useFetch("/api/admin/teams", {
  default: () => [],
});

const { data: users } = await useFetch("/api/admin/users", {
  default: () => [],
});

const form = reactive({
  id: undefined as number | undefined,
  name: "",
  ownerUserId: undefined as number | undefined,
});

const modalOpen = ref(false);

const selectedOwnerUserId = computed<number | undefined>({
  get() {
    return form.ownerUserId;
  },
  set(value: unknown) {
    if (typeof value === "number") {
      form.ownerUserId = value;
      return;
    }

    if (typeof value === "string" && value.trim()) {
      form.ownerUserId = Number(value);
      return;
    }

    form.ownerUserId = undefined;
  },
});

const columns: TableColumn<(typeof teams.value)[number]>[] = [
  { accessorKey: "name", header: "团队名称" },
  { accessorKey: "ownerName", header: "Owner" },
  { accessorKey: "memberCount", header: "成员数" },
  { accessorKey: "createdAt", header: "创建时间" },
  { id: "actions", header: "操作" },
];

function resetForm() {
  form.id = undefined;
  form.name = "";
  form.ownerUserId = undefined;
}

function openCreateModal() {
  resetForm();
  modalOpen.value = true;
}

function openEditModal(row: (typeof teams.value)[number]) {
  form.id = row.id;
  form.name = row.name;
  form.ownerUserId = row.ownerUserId ?? undefined;
  modalOpen.value = true;
}

function resolveMessage(message?: string) {
  if (!message) {
    return t("errors.generic");
  }

  return message.startsWith("errors.") ? t(message) : message;
}

async function submitTeam() {
  if (!form.name.trim()) {
    toast.add({
      title: "保存失败",
      description: t("errors.validation.teamName.required"),
      color: "error",
    });
    return;
  }

  if (!form.ownerUserId) {
    toast.add({
      title: "保存失败",
      description: t("errors.validation.ownerUserId.required"),
      color: "error",
    });
    return;
  }

  try {
    await fetchApi(
      form.id ? `/api/admin/teams/${form.id}` : "/api/admin/teams",
      {
        method: form.id ? "PATCH" : "POST",
        body: {
          name: form.name,
          ownerUserId: form.ownerUserId,
        },
      },
    );

    toast.add({
      title: form.id ? "更新成功" : "创建成功",
      color: "success",
    });

    modalOpen.value = false;
    resetForm();
    await refresh();
  } catch (e) {
    const error = e as { data?: { message?: string }; message?: string };
    toast.add({
      title: "保存失败",
      description: resolveMessage(error.data?.message || error.message),
      color: "error",
    });
  }
}

async function deleteTeam(id: number) {
  try {
    await fetchApi(`/api/admin/teams/${id}`, { method: "DELETE" });

    if (form.id === id) {
      modalOpen.value = false;
      resetForm();
    }

    await refresh();
  } catch (e) {
    const error = e as { data?: { message?: string }; message?: string };
    toast.add({
      title: "删除失败",
      description: resolveMessage(error.data?.message || error.message),
      color: "error",
    });
  }
}
</script>

<template>
  <UPageHeader title="团队管理" description="管理员可在此维护团队">
    <template #links>
      <UButton icon="i-lucide-plus" color="primary" @click="openCreateModal">
        新建团队
      </UButton>
    </template>
  </UPageHeader>
  <UPageBody>
    <UTable :data="teams" :columns="columns" :loading="pending" sticky>
      <template #ownerName-cell="{ row }">
        <span class="text-sm">{{ row.original.ownerName || "-" }}</span>
      </template>

      <template #createdAt-cell="{ row }">
        {{ new Date(row.original.createdAt).toLocaleDateString() }}
      </template>

      <template #actions-cell="{ row }">
        <div class="flex items-center gap-2">
          <UButton
            size="xs"
            variant="soft"
            icon="i-lucide-pencil"
            color="neutral"
            :aria-label="`编辑 ${row.original.name}`"
            @click="openEditModal(row.original)"
          />
          <UButton
            size="xs"
            color="error"
            variant="soft"
            icon="i-lucide-trash"
            :aria-label="`删除 ${row.original.name}`"
            @click="deleteTeam(row.original.id)"
          />
        </div>
      </template>
    </UTable>

    <UModal v-model:open="modalOpen" :title="form.id ? '编辑团队' : '新建团队'">
      <template #body>
        <UForm :state="form" class="space-y-3" @submit="submitTeam">
          <UFormField label="团队名称" name="name" required>
            <UInput
              v-model="form.name"
              class="w-full"
              placeholder="请输入团队名称"
            />
          </UFormField>

          <UFormField label="团队 Owner" name="ownerUserId" required>
            <USelectMenu
              v-model="selectedOwnerUserId"
              class="w-full"
              :items="users"
              value-key="id"
              label-key="name"
              placeholder="请选择团队 Owner"
            />
          </UFormField>

          <div class="flex items-center justify-end gap-3">
            <UButton color="neutral" variant="ghost" @click="modalOpen = false"
              >取消</UButton
            >
            <UButton
              type="submit"
              :loading="pending"
              icon="i-lucide-save"
              color="primary"
            >
              {{ form.id ? "保存修改" : "创建团队" }}
            </UButton>
          </div>
        </UForm>
      </template>
    </UModal>
  </UPageBody>
</template>
