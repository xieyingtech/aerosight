<script setup lang="ts">
definePageMeta({
  layout: team,
});

const route = useRoute();
const namespace = route.params.namespace as string;

// 获取团队详情
const {
  data: team,
  pending,
  error,
} = await useFetch(`/api/teams/${namespace}`, {
  key: `team-${namespace}-members`,
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
    // 实现添加成员的API调用
    await $fetch(`/api/teams/${namespace}/members`, {
      method: 'POST',
      body: addMemberForm
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
function confirmRemoveMember(memberId) {
  // 实现确认对话框
}

// 移除成员
async function removeMember(memberId) {
  try {
    // 实现移除成员的API调用
    await $fetch(`/api/teams/${namespace}/members/${memberId}`, {
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
      <Button 
        label="添加成员" 
        icon="i-ri-user-add-line" 
        @click="showAddMemberDialog = true"
      />
    </div>
    
    <!-- 加载状态 -->
    <div v-if="pending" class="p-6">
      <Skeleton :rows="6" animation="wave" />
    </div>

    <!-- 错误状态 -->
    <Message v-else-if="error" severity="error" :closable="false">
      <template #content>
        <div class="flex flex-column align-items-center">
          <h3>加载失败</h3>
          <p>{{ error.message }}</p>
        </div>
      </template>
    </Message>
    
    <!-- 成员列表 -->
    <Card v-else-if="team && team.memberships && team.memberships.length > 0">
      <template #header>
        <div class="flex justify-between items-center">
          <h3 class="font-bold">{{ team.name }} 的成员列表</h3>
        </div>
      </template>
      <template #content>
        <DataTable :value="team.memberships" stripedRows>
          <Column field="user.username" header="用户名" />
          <Column field="name" header="角色" />
          <Column header="权限" :style="{ width: '200px' }">
            <template #body="slotProps">
              <div class="flex flex-wrap">
                <Tag
                  v-for="(role, index) in slotProps.data.roles"
                  :key="index"
                  severity="info"
                  class="mr-1 mb-1"
                >
                  {{ role }}
                </Tag>
              </div>
            </template>
          </Column>
          <Column header="操作" :style="{ width: '150px' }">
            <template #body="slotProps">
              <Button 
                icon="i-ri-pencil-line" 
                text 
                severity="secondary" 
                aria-label="编辑" 
              />
              <Button 
                icon="i-ri-delete-bin-line" 
                text 
                severity="danger" 
                aria-label="删除" 
                @click="confirmRemoveMember(slotProps.data.id)"
              />
            </template>
          </Column>
        </DataTable>
      </template>
    </Card>
    
    <!-- 空状态 -->
    <div
      v-else
      class="empty-container flex align-items-center justify-content-center p-5 flex-column"
    >
      <i class="i-ri-user-3-line text-4xl text-gray-400 mb-3"></i>
      <span class="text-gray-400">暂无成员</span>
    </div>
    
    <!-- 添加成员对话框 -->
    <Dialog 
      v-model:visible="showAddMemberDialog"
      modal 
      header="添加团队成员" 
      :style="{ width: '30rem' }"
    >
      <div class="flex flex-col gap-4">
        <div class="flex flex-col gap-1">
          <label for="username" class="font-medium">用户名</label>
          <InputText
            id="username"
            v-model="addMemberForm.username"
            placeholder="输入用户名"
          />
        </div>

        <div class="flex flex-col gap-1">
          <label class="font-medium">角色</label>
          <MultiSelect
            v-model="addMemberForm.roles"
            :options="availableRoles"
            optionLabel="name"
            optionValue="value"
            placeholder="选择角色"
            display="chip"
          />
        </div>
      </div>
      
      <template #footer>
        <Button 
          label="取消" 
          text 
          severity="secondary" 
          @click="showAddMemberDialog = false" 
        />
        <Button 
          label="添加" 
          icon="i-ri-user-add-line"
          @click="addMember" 
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.empty-container {
  min-height: 200px;
  border-radius: 6px;
  background-color: var(--surface-ground);
}
</style>
