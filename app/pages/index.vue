<script setup lang="ts">
const appConfig = useAppConfig();
const { user, loggedIn } = useUserSession();
const { data: teams } = useFetch("/api/teams");
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
      title: "成功",
      description: "团队创建成功",
      color: "success",
    });

    // 跳转到新团队页面
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

    <!-- 创建团队的对话框 -->
    <UModal v-model="dialogVisible">
      <UCard>
        <template #header>
          <div class="flex items-center gap-2">
            <UIcon name="i-ri-team-line" class="text-xl" />
            <span class="font-bold">创建新团队</span>
          </div>
        </template>

        <div class="space-y-4">
          <p>您目前没有任何团队，创建一个新团队开始使用</p>

          <div class="space-y-4">
            <UFormField label="团队名称" :error="teamFormErrors.name">
              <UInput
                v-model="teamForm.name"
                placeholder="输入团队名称"
              />
            </UFormField>

            <UFormField label="团队标识" :error="teamFormErrors.namespace">
              <UInput
                v-model="teamForm.namespace"
                placeholder="自动生成的团队标识"
              />
              <template #help>
                标识只能包含小写字母、数字和横线(-)
              </template>
            </UFormField>

            <UFormField label="团队描述">
              <UTextarea
                v-model="teamForm.description"
                placeholder="请输入团队描述"
                :rows="3"
              />
            </UFormField>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end gap-3">
            <UButton 
              label="取消" 
              color="neutral" 
              variant="outline" 
              @click="closeDialog" 
            />
            <UButton 
              label="创建" 
              icon="i-ri-check-line" 
              @click="submitForm" 
            />
          </div>
        </template>
      </UCard>
    </UModal>
  </div>
</template>
