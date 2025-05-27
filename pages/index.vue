<script setup lang="ts">
const appConfig = useAppConfig();
const { user, loggedIn } = useUserSession();
const { data: teams } = useFetch("/api/teams");
const toast = useToast();
const confirm = useConfirm();

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
  (newName) => {
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
  // 验证表单，如果验证失败，返回false阻止对话框关闭
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
      severity: "success",
      summary: "成功",
      detail: "团队创建成功",
      life: 3000,
    });

    // 跳转到新团队页面
    navigateTo(`/teams/${response.namespace}`);
    return true;
  } catch (error: any) {
    toast.add({
      severity: "error",
      summary: "错误",
      detail: error.message || "创建团队失败",
      life: 3000,
    });
    return false;
  }
}

function navigateToDashboard() {
  const firstTeam = teams.value?.[0]?.namespace;
  if (firstTeam) {
    navigateTo(`/teams/${firstTeam}`);
  } else {
    // 显示创建团队对话框
    dialogVisible.value = true;
  }
}

// 关闭对话框
function closeDialog() {
  dialogVisible.value = false;
  // 重置表单
  teamForm.name = "";
  teamForm.namespace = "";
  teamForm.description = "";
  teamFormErrors.value = { name: "", namespace: "" };
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
          <Button
            v-if="loggedIn"
            label="进入工作台"
            icon="i-ri-dashboard-line"
            size="large"
            @click="navigateToDashboard"
          />
          <Button
            v-else
            label="立即登录"
            icon="i-ri-login-box-line"
            size="large"
            @click="navigateTo('/login')"
          />
          <Button
            label="了解更多"
            icon="i-ri-information-line"
            severity="secondary"
            outlined
            size="large"
          />
        </div>
      </div>
    </section>

    <!-- 特性区域 - 不使用卡片，直接使用网格布局 -->
    <section class="features py-8">
      <div class="container mx-auto">
        <h2 class="text-3xl font-bold text-center mb-8">主要特性</h2>
        
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <div class="text-center p-4">
            <i class="i-ri-map-pin-line text-4xl text-primary mb-2"></i>
            <h3 class="text-xl font-semibold mb-2">布防点管理</h3>
            <p>
              轻松创建和管理监控布防点，在地图上直观显示，实现精确定位和管理。
            </p>
          </div>
          
          <div class="text-center p-4">
            <i class="i-ri-team-line text-4xl text-primary mb-2"></i>
            <h3 class="text-xl font-semibold mb-2">团队协作</h3>
            <p>
              多人协作管理项目，分配任务和权限，提高团队工作效率。
            </p>
          </div>
          
          <div class="text-center p-4">
            <i class="i-ri-flight-takeoff-line text-4xl text-primary mb-2"></i>
            <h3 class="text-xl font-semibold mb-2">无人机控制</h3>
            <p>
              远程控制无人机执行监控任务，实时获取监控数据和图像。
            </p>
          </div>
        </div>
      </div>
    </section>
    
    <!-- 统计数据 -->
    <section class="stats py-8 bg-gray-50">
      <div class="container mx-auto">
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <Card>
            <template #title>
              <h3 class="text-xl font-semibold flex flex-col items-center">
                <i class="i-ri-map-pin-fill text-4xl text-primary mb-2"></i>
                <span class="text-4xl font-bold text-primary">500+</span>
              </h3>
            </template>
            <template #content>
              <p class="m-0 text-center text-xl">
                监控点
              </p>
            </template>
          </Card>
          
          <Card>
            <template #title>
              <h3 class="text-xl font-semibold flex flex-col items-center">
                <i class="i-ri-user-fill text-4xl text-primary mb-2"></i>
                <span class="text-4xl font-bold text-primary">100+</span>
              </h3>
            </template>
            <template #content>
              <p class="m-0 text-center text-xl">
                客户
              </p>
            </template>
          </Card>
          
          <Card>
            <template #title>
              <h3 class="text-xl font-semibold flex flex-col items-center">
                <i class="i-ri-flight-takeoff-fill text-4xl text-primary mb-2"></i>
                <span class="text-4xl font-bold text-primary">50+</span>
              </h3>
            </template>
            <template #content>
              <p class="m-0 text-center text-xl">
                无人机
              </p>
            </template>
          </Card>
        </div>
      </div>
    </section>

    <!-- 客户案例 -->
    <section class="customers py-8">
      <div class="container mx-auto">
        <h2 class="text-3xl font-bold text-center mb-8">谁在使用</h2>

        <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-4">
          <div v-for="i in 8" :key="i" class="p-3">
            <div
              class="p-4 border rounded flex align-items-center justify-content-center h-24"
            >
              <span class="text-xl text-gray-400">客户 {{ i }}</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 底部CTA -->
    <section class="cta py-8 bg-primary text-center">
      <div class="container mx-auto">
        <h2 class="text-3xl font-bold mb-4">准备好开始了吗？</h2>
        <p class="mb-6">立即加入我们，开启智能无人机监控之旅</p>

        <div class="flex justify-center gap-3">
          <Button
            v-if="loggedIn"
            label="进入工作台"
            icon="i-ri-dashboard-line"
            size="large"
            @click="navigateToDashboard"
          />
          <Button
            v-else
            label="立即登录"
            icon="i-ri-login-box-line"
            size="large"
            @click="navigateTo('/login')"
          />
        </div>
      </div>
    </section>

    <!-- 创建团队的对话框 -->
    <Dialog
      v-model:visible="dialogVisible"
      modal
      header="创建团队"
      :style="{ width: '30rem' }"
      :closable="false"
    >
      <template #header>
        <div class="inline-flex items-center justify-center gap-2">
          <i class="i-ri-team-line text-2xl"></i>
          <span class="font-bold">创建新团队</span>
        </div>
      </template>

      <p class="mb-4">您目前没有任何团队，创建一个新团队开始使用</p>

      <div class="flex flex-col gap-4">
        <div class="flex flex-col gap-1">
          <label for="teamName" class="font-medium">团队名称</label>
          <InputText
            id="teamName"
            v-model="teamForm.name"
            placeholder="输入团队名称"
            :class="{ 'p-invalid': teamFormErrors.name }"
          />
          <small v-if="teamFormErrors.name" class="p-error">{{
            teamFormErrors.name
          }}</small>
        </div>

        <div class="flex flex-col gap-1">
          <label for="namespace" class="font-medium">团队标识</label>
          <InputText
            id="namespace"
            v-model="teamForm.namespace"
            placeholder="自动生成的团队标识"
            :class="{ 'p-invalid': teamFormErrors.namespace }"
          />
          <small class="text-xs text-gray-500"
            >标识只能包含小写字母、数字和横线(-)</small
          >
          <small v-if="teamFormErrors.namespace" class="p-error">{{
            teamFormErrors.namespace
          }}</small>
        </div>

        <div class="flex flex-col gap-1">
          <label for="description" class="font-medium">团队描述</label>
          <Textarea
            id="description"
            v-model="teamForm.description"
            placeholder="请输入团队描述"
            rows="3"
            autoResize
          />
        </div>
      </div>

      <template #footer>
        <Button label="取消" text severity="secondary" @click="closeDialog" />
        <Button label="创建" icon="i-ri-check-line" @click="submitForm" />
      </template>
    </Dialog>

    <Toast />
  </div>
</template>
