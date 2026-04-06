<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";

const { t } = useI18n();
const toast = useToast();
const fetchApi = $fetch;

const {
  data: users,
  pending,
  refresh,
} = await useFetch("/api/admin/users", {
  default: () => [],
});

const form = reactive({
  id: undefined as number | undefined,
  name: "",
  email: "",
  phone: "",
  role: "user" as "user" | "admin",
  password: "",
});

const modalOpen = ref(false);

const columns: TableColumn<(typeof users.value)[number]>[] = [
  { accessorKey: "name", header: "姓名" },
  { accessorKey: "email", header: "邮箱" },
  { accessorKey: "phone", header: "手机号" },
  { accessorKey: "role", header: "角色" },
  { accessorKey: "createdAt", header: "创建时间" },
  { id: "actions", header: "操作" },
];

function resetForm() {
  form.id = undefined;
  form.name = "";
  form.email = "";
  form.phone = "";
  form.role = "user";
  form.password = "";
}

function openCreateModal() {
  resetForm();
  modalOpen.value = true;
}

function openEditModal(row: (typeof users.value)[number]) {
  form.id = row.id;
  form.name = row.name;
  form.email = row.email || "";
  form.phone = row.phone || "";
  form.role = row.role;
  form.password = "";
  modalOpen.value = true;
}

function resolveMessage(message?: string) {
  if (!message) {
    return t("errors.generic");
  }

  return message.startsWith("errors.") ? t(message) : message;
}

async function submitUser() {
  if (!form.name.trim()) {
    toast.add({
      title: "保存失败",
      description: t("errors.validation.userName.required"),
      color: "error",
    });
    return;
  }

  if (!form.id && !form.password.trim()) {
    toast.add({
      title: "保存失败",
      description: t("errors.validation.password.required"),
      color: "error",
    });
    return;
  }

  try {
    await fetchApi(
      form.id ? `/api/admin/users/${form.id}` : "/api/admin/users",
      {
        method: form.id ? "PATCH" : "POST",
        body: {
          name: form.name,
          email: form.email,
          phone: form.phone,
          role: form.role,
          ...(form.password.trim() ? { password: form.password } : {}),
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

async function deleteUser(id: number) {
  try {
    await fetchApi(`/api/admin/users/${id}`, { method: "DELETE" });

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
  <UPageHeader title="用户管理" description="管理员可在此维护系统用户">
    <template #links>
      <UButton icon="i-lucide-plus" color="primary" @click="openCreateModal">
        新建用户
      </UButton>
    </template>
  </UPageHeader>
  <UPageBody>
    <UTable :data="users" :columns="columns" :loading="pending" sticky>
      <template #role-cell="{ row }">
        <UBadge
          :color="row.original.role === 'admin' ? 'warning' : 'neutral'"
          variant="subtle"
        >
          {{ row.original.role === "admin" ? "管理员" : "普通用户" }}
        </UBadge>
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
            @click="deleteUser(row.original.id)"
          />
        </div>
      </template>
    </UTable>

    <UModal v-model:open="modalOpen" :title="form.id ? '编辑用户' : '新建用户'">
      <template #body>
        <UForm
          :state="form"
          class="grid gap-3 md:grid-cols-2"
          @submit="submitUser"
        >
          <UFormField label="姓名" name="name" required>
            <UInput
              v-model="form.name"
              class="w-full"
              placeholder="请输入姓名"
            />
          </UFormField>

          <UFormField label="角色" name="role" required>
            <USelectMenu
              v-model="form.role"
              class="w-full"
              :items="[
                { label: '普通用户', value: 'user' },
                { label: '管理员', value: 'admin' },
              ]"
              value-key="value"
              label-key="label"
            />
          </UFormField>

          <UFormField label="邮箱" name="email">
            <UInput v-model="form.email" class="w-full" placeholder="可选" />
          </UFormField>

          <UFormField label="手机号" name="phone">
            <UInput v-model="form.phone" class="w-full" placeholder="可选" />
          </UFormField>

          <UFormField
            label="密码"
            name="password"
            :required="!form.id"
            class="md:col-span-2"
          >
            <UInput
              v-model="form.password"
              type="password"
              class="w-full"
              :placeholder="form.id ? '留空则不修改密码' : '请输入登录密码'"
            />
          </UFormField>

          <div class="md:col-span-2 flex items-center justify-end gap-3">
            <UButton color="neutral" variant="ghost" @click="modalOpen = false"
              >取消</UButton
            >
            <UButton
              type="submit"
              icon="i-lucide-save"
              color="primary"
              :loading="pending"
            >
              {{ form.id ? "保存修改" : "创建用户" }}
            </UButton>
          </div>
        </UForm>
      </template>
    </UModal>
  </UPageBody>
</template>
