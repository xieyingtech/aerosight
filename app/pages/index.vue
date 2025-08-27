<script setup lang="ts">
import { onMounted } from 'vue';
onMounted(() => {
  refreshTeams();
});
const appConfig = useAppConfig();
const { user, loggedIn } = useUserSession();
const { data: teams, refresh: refreshTeams, pending: teamsPending } = useAsyncData("teams", () => $fetch("/api/teams"));
const toast = useToast();

// 对话框可见性控制
const dialogVisible = ref(false);

// 表单数据
const teamForm = reactive({
  name: "",
  namespace: "",
  description: "",
});

// 表单验证
const teamFormErrors = ref({
  name: "",
  namespace: "",
});

// 生成namespace的函数
function generateNamespace(name: string) {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, "")
    .replace(/\s+/g, "-")
    .replace(/^-+|-+$/g, "");
}

// 监听名称变化，自动生成namespace
watch(
  () => teamForm.name,
  (newName: string) => {
    if (newName) {
      teamForm.namespace = generateNamespace(newName);
    }
  }
);

// 验证表单
function validateTeamForm() {
  let isValid = true;
  teamFormErrors.value = { name: "", namespace: "" };

  if (!teamForm.name.trim()) {
    teamFormErrors.value.name = "请输入团队名称";
    isValid = false;
  }

  if (!teamForm.namespace.trim()) {
    teamFormErrors.value.namespace = "请输入团队标识";
    isValid = false;
  } else if (!/^[a-z0-9-]+$/.test(teamForm.namespace)) {
    teamFormErrors.value.namespace = "团队标识只能包含小写字母、数字和横线";
    isValid = false;
  }

  return isValid;
}

// 创建团队
async function createTeam() {
  if (!validateTeamForm()) return false;
  try {
    const response = await $fetch("/api/teams", {
      method: "POST",
      body: {
        name: teamForm.name,
        namespace: teamForm.namespace,
        description: teamForm.description,
      },
    });
    toast.add({
      title: "成功",
      description: "团队创建成功",
      color: "success",
    });
    closeDialog();
    await refreshTeams();
    navigateTo(`/teams/${response.namespace}`);
    return true;
  } catch (error: any) {
    toast.add({
      title: "错误",
      description: error.message || "创建团队失败",
      color: "error",
    });
    return false;
  }
}

function navigateToDashboard() {
  if (teamsPending.value) return;
  if (!teams.value || !Array.isArray(teams.value)) {
    toast.add({ title: "错误", description: "团队数据获取失败", color: "error" });
    return;
  }
  if (teams.value.length > 0) {
    navigateTo(`/teams/${teams.value[0]?.namespace ?? ''}`);
  } else {
    dialogVisible.value = true;
  }
}

// 关闭对话框
function closeDialog() {
  dialogVisible.value = false;
  teamForm.name = "";
  teamForm.namespace = "";
  teamForm.description = "";
  teamFormErrors.value = { name: "", namespace: "" };
  refreshTeams(); // 弹窗关闭后刷新团队列表，确保数据同步
}

// 提交表单
async function submitForm() {
  const result = await createTeam();
  if (result) {
    closeDialog();
  }
}
</script>

<template>
  <div class="homepage max-w-screen-lg mx-auto px-4">
    <!-- 顶部区域 - 保持不变 -->
    <section class="hero py-8 px-4 text-center">
      <div class="container mx-auto">
        <h1 class="text-5xl font-bold mb-4">{{ appConfig.site.title }}</h1>
        <p class="text-xl mb-6 line-height-3 max-w-3xl mx-auto">
          {{ appConfig.site.description || "智能无人机监控管理系统" }}
        </p>

        <div class="flex justify-center gap-3 mt-8">
          <UButton
            v-if="loggedIn"
            label="进入工作台"
            icon="i-ri-dashboard-line"
            size="lg"
            @click="navigateToDashboard"
          />
          <UButton
            v-else
            label="立即登录"
            icon="i-ri-login-box-line"
            size="lg"
            @click="navigateTo('/login')"
          />
          <UButton
            label="了解更多"
            icon="i-ri-information-line"
            color="neutral"
            variant="outline"
            size="lg"
          />
        </div>
      </div>
    </section>

    <!-- 特性区域 - 不使用卡片，直接使用网格布局 -->
    <USection class="features" padding="lg">
      <h2 class="text-3xl font-bold text-center mb-8 text-gray-900 dark:text-gray-100">主要特性</h2>
      <UGrid cols="1 md:2 lg:3" gap="4">
        <div class="text-center p-4">
          <UIcon name="i-ri-map-pin-line" class="text-4xl text-primary mb-2" />
          <h3 class="text-xl font-semibold mb-2">布防点管理</h3>
          <p>轻松创建和管理监控布防点，在地图上直观显示，实现精确定位和管理。</p>
        </div>
        <div class="text-center p-4">
          <UIcon name="i-ri-team-line" class="text-4xl text-primary mb-2" />
          <h3 class="text-xl font-semibold mb-2">团队协作</h3>
          <p>多人协作管理项目，分配任务和权限，提高团队工作效率。</p>
        </div>
        <div class="text-center p-4">
          <UIcon name="i-ri-flight-takeoff-line" class="text-4xl text-primary mb-2" />
          <h3 class="text-xl font-semibold mb-2">无人机控制</h3>
          <p>远程控制无人机执行监控任务，实时获取监控数据和图像。</p>
        </div>
      </UGrid>
    </USection>
    
    <!-- 统计数据 -->
    <section class="stats py-8 bg-gray-50 dark:bg-gray-900">
      <div class="container mx-auto">
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <UCard>
            <template #header>
              <div class="text-center">
                <UIcon name="i-ri-map-pin-fill" class="text-4xl text-primary mb-2" />
                <div class="text-4xl font-bold text-primary">500+</div>
              </div>
            </template>
            <div class="text-center text-xl">
              监控点
            </div>
          </UCard>
          
          <UCard>
            <template #header>
              <div class="text-center">
                <UIcon name="i-ri-user-fill" class="text-4xl text-primary mb-2" />
                <div class="text-4xl font-bold text-primary">100+</div>
              </div>
            </template>
            <div class="text-center text-xl">
              客户
            </div>
          </UCard>
          
          <UCard>
            <template #header>
              <div class="text-center">
                <UIcon name="i-ri-flight-takeoff-fill" class="text-4xl text-primary mb-2" />
                <div class="text-4xl font-bold text-primary">50+</div>
              </div>
            </template>
            <div class="text-center text-xl">
              无人机
            </div>
          </UCard>
        </div>
      </div>
    </section>

    <!-- 客户案例 -->
    <USection class="customers" padding="lg">
      <h2 class="text-3xl font-bold text-center mb-8">谁在使用</h2>
      <UGrid cols="1 sm:2 md:4" gap="4">
        <div v-for="i in 8" :key="i" class="p-3">
          <UCard class="flex items-center justify-center h-24">
            <span class="text-xl text-gray-400 dark:text-gray-300">客户 {{ i }}</span>
          </UCard>
        </div>
      </UGrid>
    </USection>

    <!-- 底部CTA -->
    <section class="cta py-8 bg-primary dark:bg-primary-900 text-center">
      <div class="container mx-auto">
        <h2 class="text-3xl font-bold mb-4 text-white dark:text-gray-100">准备好开始了吗？</h2>
        <p class="mb-6 text-white dark:text-gray-200">立即加入我们，开启智能无人机监控之旅</p>

        <div class="flex justify-center gap-3">
          <UButton
            v-if="loggedIn"
            label="进入工作台"
            icon="i-ri-dashboard-line"
            size="lg"
            @click="navigateToDashboard"
          />
          <UButton
            v-else
            label="立即登录"
            icon="i-ri-login-box-line"
            size="lg"
            @click="navigateTo('/login')"
          />
        </div>
      </div>
    </section>

    <!-- 页面底部留白50px，防止底部按钮遮挡内容 -->
    <div style="height:50px;"></div>

    <UDrawer title="创建团队" v-model="dialogVisible">
      <div class="fixed bottom-12 left-0 w-full flex justify-center z-50">
        <UButton icon="i-lucide-rocket" size="xl" color="primary" variant="solid"@click="dialogVisible = true">立即开始
        </UButton>
      </div>
      <template #body>
        <div class="flex flex-col items-center justify-center w-full py-6">
          <UCard class="w-full max-w-md shadow-lg">
            <template #header>
              <div class="flex items-center gap-2 justify-center">
                <UIcon name="i-ri-team-line" class="text-2xl text-primary" />
                <span class="font-bold text-lg">创建团队</span>
              </div>
            </template>
            <UForm :state="teamForm" @submit="async (e) => { await submitForm(); }" class="space-y-5">
              <UFormField label="团队名称" required :error="teamFormErrors.name">
                <UInput v-model="teamForm.name" placeholder="输入团队名称" size="lg" class="w-full" />
              </UFormField>
              <UFormField label="团队标识（自动生成）" required :error="teamFormErrors.namespace">
                <UInput v-model="teamForm.namespace" placeholder="自动生成标识" readonly size="lg" class="w-full" />
                <template #help>标识仅含小写字母、数字和横线(如 "team-alpha-01")</template>
              </UFormField>
              <UFormField label="团队描述">
                <UTextarea v-model="teamForm.description" placeholder="描述团队目标或职责" :rows="3" size="lg" class="w-full" />
              </UFormField>
              <div class="flex justify-center gap-4 pt-2">
                <UButton label="取消" color="neutral" size="lg" variant="outline" @click="closeDialog" />
                <UButton 
                  type="submit" 
                  label="创建" 
                  icon="i-heroicons-check"
                  size="lg"
                />
              </div>
            </UForm>
          </UCard>
        </div>
      </template>
    </UDrawer>

  </div>
</template>
