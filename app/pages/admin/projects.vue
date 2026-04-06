<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";

const { t } = useI18n();
const toast = useToast();
const fetchApi = $fetch;

const {
  data: projects,
  pending,
  refresh,
} = await useFetch("/api/admin/projects", {
  default: () => [],
});

const { data: teamOptions } = await useFetch("/api/admin/teams", {
  default: () => [],
});

const form = reactive({
  id: undefined as number | undefined,
  teamId: undefined as number | undefined,
  name: "",
  description: "",
});

const modalOpen = ref(false);

const selectedTeamId = computed<number | undefined>({
  get() {
    return form.teamId;
  },
  set(value: unknown) {
    if (typeof value === "number") {
      form.teamId = value;
      return;
    }

    if (typeof value === "string" && value.trim()) {
      form.teamId = Number(value);
      return;
    }

    form.teamId = undefined;
  },
});

const columns: TableColumn<(typeof projects.value)[number]>[] = [
  { accessorKey: "name", header: "项目名称" },
  { accessorKey: "teamName", header: "所属团队" },
  { accessorKey: "createdByUserName", header: "创建人" },
  { accessorKey: "updatedAt", header: "最近更新" },
  { id: "actions", header: "操作" },
];

function resetForm() {
  form.id = undefined;
  form.teamId = undefined;
  form.name = "";
  form.description = "";
}

function openCreateModal() {
  resetForm();
  modalOpen.value = true;
}

function openEditModal(row: (typeof projects.value)[number]) {
  form.id = row.id;
  form.teamId = row.teamId;
  form.name = row.name;
  form.description = row.description || "";
  modalOpen.value = true;
}

function resolveMessage(message?: string) {
  if (!message) {
    return t("errors.generic");
  }

  return message.startsWith("errors.") ? t(message) : message;
}

async function submitProject() {
  if (!form.teamId) {
    toast.add({
      title: "保存失败",
      description: t("errors.validation.teamId.required"),
      color: "error",
    });
    return;
  }

  if (!form.name.trim()) {
    toast.add({
      title: "保存失败",
      description: t("errors.validation.projectName.required"),
      color: "error",
    });
    return;
  }

  try {
    await fetchApi(
      form.id ? `/api/admin/projects/${form.id}` : "/api/admin/projects",
      {
        method: form.id ? "PATCH" : "POST",
        body: {
          teamId: form.teamId,
          name: form.name,
          description: form.description,
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

async function deleteProject(id: number) {
  try {
    await fetchApi(`/api/admin/projects/${id}`, { method: "DELETE" });

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
  <UPageHeader title="项目管理" description="管理员可在此维护项目">
    <template #links>
      <UButton icon="i-lucide-plus" color="primary" @click="openCreateModal">
        新建项目
      </UButton>
    </template>
  </UPageHeader>
  <UPageBody>
    <UTable :data="projects" :columns="columns" :loading="pending" sticky>
      <template #updatedAt-cell="{ row }">
        {{ new Date(row.original.updatedAt).toLocaleDateString() }}
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
            @click="deleteProject(row.original.id)"
          />
        </div>
      </template>
    </UTable>

    <UModal v-model:open="modalOpen" :title="form.id ? '编辑项目' : '新建项目'">
      <template #body>
        <UForm
          :state="form"
          class="grid gap-3 md:grid-cols-2"
          @submit="submitProject"
        >
          <UFormField label="所属团队" name="teamId" required>
            <USelectMenu
              v-model="selectedTeamId"
              class="w-full"
              :items="teamOptions"
              value-key="id"
              label-key="name"
              placeholder="请选择团队"
            />
          </UFormField>

          <UFormField label="项目名称" name="name" required>
            <UInput
              v-model="form.name"
              class="w-full"
              placeholder="请输入项目名称"
            />
          </UFormField>

          <UFormField label="项目描述" name="description" class="md:col-span-2">
            <UTextarea
              v-model="form.description"
              class="w-full"
              :rows="3"
              placeholder="可选"
            />
          </UFormField>

          <div class="md:col-span-2 flex items-center justify-end gap-3">
            <UButton color="neutral" variant="ghost" @click="modalOpen = false"
              >取消</UButton
            >
            <UButton
              type="submit"
              :loading="pending"
              icon="i-lucide-save"
              color="primary"
            >
              {{ form.id ? "保存修改" : "创建项目" }}
            </UButton>
          </div>
        </UForm>
      </template>
    </UModal>
  </UPageBody>
</template>
