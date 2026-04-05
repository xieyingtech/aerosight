<script setup lang="ts">
import type { NavigationMenuItem } from "@nuxt/ui";

const route = useRoute();
const { user } = useUserSession();

if (!user.value) {
	navigateTo("/login");
}

const projectId = computed(() => String(route.params.id || ""));
const projectBasePath = computed(() => {
	if (!projectId.value) {
		return "/projects";
	}

	return `/projects/${projectId.value}`;
});

const navItems = computed(() => {
	const items = [
		{
			label: "数据看板",
			icon: "i-lucide-layout-dashboard",
			to: projectBasePath.value,
		},
		{ label: "业务", type: "label" },
		{
			label: "设备监控",
			icon: "i-lucide-radio",
			to: `${projectBasePath.value}/devices`,
		},
		{
			label: "智能体",
			icon: "i-lucide-bot",
			to: `${projectBasePath.value}/agents`,
		},
		{
			label: "任务编排",
			icon: "i-lucide-list-todo",
			to: `${projectBasePath.value}/tasks`,
		},
		{
			label: "问题中心",
			icon: "i-lucide-bug",
			to: `${projectBasePath.value}/issues`,
		},
		{
			label: "素材库",
			icon: "i-lucide-folder-open",
			to: `${projectBasePath.value}/assets`,
		},
	] as NavigationMenuItem[];

	if (user.value?.role === "admin") {
		items.push(
			{ label: "管理", type: "label" },
			{
				label: "团队成员",
				icon: "i-lucide-users",
				to: `${projectBasePath.value}/teams`,
			},
			{
				label: "系统设置",
				icon: "i-lucide-settings",
				to: `${projectBasePath.value}/settings`,
			},
		);
	}

	return items;
});
</script>

<template>
	<UPage>
		<template #left>
			<UPageAside>
				<UNavigationMenu :items="navItems" orientation="vertical" />
			</UPageAside>
		</template>

		<UPageBody>
			<UPageHeader
				title="项目控制台"
				:description="`当前项目 ID：${projectId}`"
			/>
		</UPageBody>
	</UPage>
</template>
