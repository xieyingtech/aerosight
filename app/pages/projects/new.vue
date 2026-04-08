<script setup lang="ts">
import { z } from "zod";

const { t } = useI18n();
const toast = useToast();
const fetchApi = $fetch;

const schema = z.object({
  teamId: z.coerce
    .number()
    .int()
    .positive(t("errors.validation.teamId.required")),
  name: z.string().trim().min(1, t("errors.validation.projectName.required")),
});

const state = reactive<{ teamId: number | undefined; name: string }>({
  teamId: undefined,
  name: "",
});

const { data: teamOptions, pending: teamsPending } = await useFetch(
  "/api/teams/managed",
  {
    default: () => [],
  },
);

const selectedTeamId = computed<number | undefined>({
  get() {
    return state.teamId;
  },
  set(value: unknown) {
    if (typeof value === "number") {
      state.teamId = value;
      return;
    }

    if (typeof value === "string" && value.trim()) {
      state.teamId = Number(value);
      return;
    }

    state.teamId = undefined;
  },
});
</script>

<template>
  <UPageHeader
    :title="t('projects.new.title')"
    :description="t('projects.new.description')"
  />

  <UPageBody>
    <UCard>
      <UForm
        :schema="schema"
        :state="state"
        class="space-y-4"
        @submit="
          async (payload) => {
            try {
              const created = await fetchApi('/api/projects', {
                method: 'POST',
                body: {
                  teamId: payload.data.teamId,
                  name: payload.data.name,
                },
              });

              const projectId = created[0]?.id;

              toast.add({
                title: t('projects.new.successTitle'),
                description: t('projects.new.successDescription'),
                color: 'success',
              });

              await navigateTo(
                projectId ? `/projects/${projectId}` : '/projects',
              );
            } catch (e) {
              const error = e as {
                data?: { message?: string };
                message?: string;
              };
              const message = error.data?.message || error.message;

              toast.add({
                title: t('projects.new.failedTitle'),
                description: message
                  ? String(message).startsWith('errors.')
                    ? t(String(message))
                    : String(message)
                  : t('errors.generic'),
                color: 'error',
              });
            }
          }
        "
      >
        <UFormField
          :label="t('projects.new.fields.team.label')"
          name="teamId"
          required
        >
          <USelectMenu
            v-model="selectedTeamId"
            :items="teamOptions"
            value-key="id"
            label-key="name"
            :placeholder="t('projects.new.fields.team.placeholder')"
            :loading="teamsPending"
            class="w-full"
          />
        </UFormField>

        <UFormField
          :label="t('projects.new.fields.name.label')"
          name="name"
          required
        >
          <UInput
            v-model="state.name"
            class="w-full"
            :placeholder="t('projects.new.fields.name.placeholder')"
          />
        </UFormField>

        <UAlert
          v-if="!teamsPending && !teamOptions.length"
          color="warning"
          variant="subtle"
          :title="t('projects.new.noTeamTitle')"
          :description="t('projects.new.noTeamDescription')"
        />

        <div class="flex items-center gap-3">
          <UButton
            type="submit"
            color="primary"
            icon="i-lucide-plus"
            :disabled="!teamOptions.length"
          >
            {{ t("projects.new.submit") }}
          </UButton>
          <UButton color="neutral" variant="ghost" to="/projects">
            {{ t("projects.new.cancel") }}
          </UButton>
        </div>
      </UForm>
    </UCard>
  </UPageBody>
</template>
