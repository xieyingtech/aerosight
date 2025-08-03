<script setup lang="ts">
definePageMeta({
  layout: "team",
});

const route = useRoute();
const namespace = route.params.namespace as string;

// 获取团队详情
const {
  data: team,
  pending,
  error,
} = await useFetch(`/api/teams/${namespace}`, {
  key: `team-${namespace}-members`, // Key can remain, or be updated for clarity
});

// 添加成员对话框控制
const showAddMemberDialog = ref(false);

// 添加成员表单
const addMemberForm = reactive({
  username: '',
  roles: []
});

// 可选角色列表
const availableRoles = ref([
  { name: '管理员', value: 'admin' },
  { name: '成员', value: 'member' },
  { name: '只读', value: 'readonly' }
]);

// 表单验证
function validateAddMemberForm() {
  // 实现表单验证逻辑
  return true;
}

// 添加成员
async function addMember() {
  if (!validateAddMemberForm()) return;
  
  try {
    // API endpoint for adding members needs to be aware of the new TeamMember/MemberRole structure
    await $fetch(`/api/teams/${namespace}/members`, {
      method: 'POST',
      body: {
        username: addMemberForm.username, // Or userId
        roles: addMemberForm.roles, // Array of role names/IDs
      }
    });
    
    // 刷新数据
    await refreshNuxtData(`team-${namespace}-members`);
    
    // 关闭对话框
    showAddMemberDialog.value = false;
  } catch (error) {
    // 处理错误
    console.error('添加成员失败', error);
  }
}

// 移除成员确认
function confirmRemoveMember(memberId: number) {
  // 实现确认对话框
  if (confirm('确定要移除该成员吗？')) {
    removeMember(memberId);
  }
}

// 表格列定义
const columns = [
  {
    key: 'username',
    label: '用户名'
  },
  {
    key: 'roles',
    label: '角色'
  },
  {
    key: 'actions',
    label: '操作'
  }
];

// 移除成员
async function removeMember(teamMemberId: number) { // Assuming memberId is now teamMemberId
  try {
    // API endpoint for removing members
    await $fetch(`/api/teams/${namespace}/members/${teamMemberId}`, {
      method: 'DELETE'
    });
    
    // 刷新数据
    await refreshNuxtData(`team-${namespace}-members`);
  } catch (error) {
    // 处理错误
    console.error('移除成员失败', error);
  }
}
</script>

<template>
  <div class="p-4">
    <div class="flex justify-between items-center mb-4">
      <h1 class="text-2xl font-bold">团队成员管理</h1>
      <UButton 
        label="添加成员" 
        icon="i-ri-user-add-line" 
        @click="showAddMemberDialog = true"
      />
    </div>
    
    <!-- 加载状态 -->
    <div v-if="pending" class="p-6">
      <USkeleton class="h-4 w-full" />
      <USkeleton class="h-4 w-4/5 mt-2" />
      <USkeleton class="h-4 w-3/5 mt-2" />
      <USkeleton class="h-4 w-2/3 mt-2" />
      <USkeleton class="h-4 w-1/2 mt-2" />
      <USkeleton class="h-4 w-3/4 mt-2" />
    </div>

    <!-- 错误状态 -->
    <UAlert v-else-if="error" color="error" variant="solid" :close-button="false">
      <template #title>加载失败</template>
      <template #description>{{ error.message }}</template>
    </UAlert>
    
    <!-- 成员列表 -->
    <UCard v-else-if="team && team.teamMembers && team.teamMembers.length > 0">
      <template #header>
        <div class="flex justify-between items-center">
          <h3 class="font-bold">{{ team.name }} 的成员列表</h3>
        </div>
      </template>
      <div class="p-4">
        <UTable :rows="team.teamMembers">
          <template #username-data="{ row }">
            {{ (row as any).user?.username }}
          </template>
          <template #roles-data="{ row }">
            <div class="flex flex-wrap gap-1">
              <UBadge
                v-for="(roleName, index) in (row as any).roles"
                :key="index"
                color="primary"
                variant="subtle"
              >
                {{ roleName }}
              </UBadge>
            </div>
          </template>
          <template #actions-data="{ row }">
            <div class="flex gap-1">
              <UButton 
                icon="i-ri-pencil-line" 
                variant="ghost" 
                color="neutral"
                size="sm"
                aria-label="编辑" 
              />
              <UButton 
                icon="i-ri-delete-bin-line" 
                variant="ghost" 
                color="error"
                size="sm"
                aria-label="删除" 
                @click="confirmRemoveMember(Number((row as any).id))" 
              />
            </div>
          </template>
        </UTable>
      </div>
    </UCard>
    
    <!-- 空状态 -->
    <div
      v-else
      class="empty-container flex items-center justify-center p-5 flex-col"
    >
      <UIcon name="i-ri-user-3-line" class="text-4xl text-gray-400 mb-3" />
      <span class="text-gray-400">暂无成员</span>
    </div>
    
    <!-- 添加成员对话框 -->
    <UModal v-model="showAddMemberDialog">
      <UCard>
        <template #header>
          <div class="flex items-center gap-2">
            <UIcon name="i-ri-user-add-line" />
            <span class="font-bold">添加团队成员</span>
          </div>
        </template>
        <div class="space-y-4">
          <UFormField label="用户名">
            <UInput
              v-model="addMemberForm.username"
              placeholder="输入用户名"
            />
          </UFormField>

          <UFormField label="角色">
            <USelectMenu
              v-model="addMemberForm.roles"
              :options="availableRoles"
              option-attribute="name"
              value-attribute="value"
              placeholder="选择角色"
              multiple
            />
          </UFormField>
        </div>
        
        <template #footer>
          <div class="flex justify-end gap-3">
            <UButton 
              label="取消" 
              color="neutral"
              variant="outline"
              @click="showAddMemberDialog = false" 
            />
            <UButton 
              label="添加" 
              icon="i-ri-user-add-line"
              @click="addMember" 
            />
          </div>
        </template>
      </UCard>
    </UModal>
  </div>
</template>

<style scoped>
.empty-container {
  min-height: 200px;
  border-radius: 6px;
  background-color: var(--surface-ground);
}
</style>
