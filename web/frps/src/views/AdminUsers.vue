<template>
  <div class="ent-page">
    <div class="page-header">
      <div class="title-section">
        <h1 class="page-title">用户</h1>
        <p class="page-subtitle">
          一个本地账号，其余通过 SSO
          登录并按用户名匹配。未登记的用户首次登录后需要其他管理员批准。
        </p>
      </div>
      <div v-if="data?.oidcEnabled" class="header-actions">
        <el-input
          v-model="newUser"
          placeholder="SSO 用户名"
          class="name-input"
          clearable
          @keyup.enter="onAdd"
        />
        <el-button type="primary" @click="onAdd">登记用户</el-button>
      </div>
    </div>

    <el-card v-if="data" shadow="never" class="panel">
      <template #header>本地账号</template>
      <div class="local">
        <div>
          <div class="local-name">{{ data.local.username }}</div>
          <div class="muted">
            二次验证：
            <el-tag v-if="data.local.totpEnabled" type="success" size="small"
              >已开启</el-tag
            >
            <el-tag v-else type="warning" size="small"
              >未设置（下次登录时设置）</el-tag
            >
            <span v-if="data.local.totpEnabled"
              >· 剩余恢复密钥 {{ data.local.recoveryLeft }} 个</span
            >
          </div>
          <div class="muted">
            最后登录：{{ fmt(data.local.lastLoginAt) }}
            {{ data.local.lastLoginIP || '' }}
          </div>
        </div>
        <div class="local-actions">
          <el-button
            v-if="session?.source === 'local' && data.local.totpEnabled"
            @click="onRegenerate"
          >
            重新生成恢复密钥
          </el-button>
          <el-button
            v-if="data.local.totpEnabled"
            type="danger"
            plain
            @click="onReset"
            >重置二次验证</el-button
          >
        </div>
      </div>
    </el-card>

    <el-alert
      v-if="data && !data.oidcEnabled"
      type="info"
      :closable="false"
      title="未配置 SSO"
      description="在 frps 配置中设置 enterprise.admin.oidc 后，其他管理员可以通过 SSO 登录。"
    />

    <el-card v-if="data?.oidcEnabled" shadow="never" class="panel">
      <template #header>
        SSO 用户
        <el-tag v-if="pending > 0" type="warning" size="small" class="count"
          >{{ pending }} 个待批准</el-tag
        >
      </template>
      <el-table :data="data.users" empty-text="还没有 SSO 用户">
        <el-table-column label="用户名" min-width="160">
          <template #default="{ row }">
            <div class="name">{{ row.username }}</div>
            <div class="muted">{{ row.email || '' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)" disable-transitions>{{
              statusLabel(row.status)
            }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="申请 / 处理" min-width="200">
          <template #default="{ row }">
            <div v-if="row.requestedAt" class="muted">
              申请：{{ fmt(row.requestedAt) }}
            </div>
            <div v-if="row.decidedBy" class="muted">
              {{ row.decidedBy }} 于 {{ fmt(row.decidedAt) }}
            </div>
          </template>
        </el-table-column>
        <el-table-column label="最后登录" min-width="170">
          <template #default="{ row }">
            <div>{{ fmt(row.lastLoginAt) }}</div>
            <div class="muted">{{ row.lastLoginIP || '' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="备注" min-width="120">
          <template #default="{ row }">
            <span class="note" @click="onNote(row)">{{ row.note || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <template v-if="row.status === 'pending'">
              <el-button
                size="small"
                type="success"
                @click="act(row, 'approve')"
                >批准</el-button
              >
              <el-button size="small" type="danger" @click="act(row, 'reject')"
                >拒绝</el-button
              >
            </template>
            <el-button
              v-else-if="row.status === 'approved'"
              size="small"
              :disabled="row.username === session?.username"
              @click="act(row, 'disable')"
            >
              停用
            </el-button>
            <el-button
              v-else
              size="small"
              type="success"
              @click="act(row, 'enable')"
              >启用</el-button
            >
            <el-button
              size="small"
              type="danger"
              plain
              :disabled="row.username === session?.username"
              @click="onDelete(row)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="showCodes" title="新的恢复密钥" width="420px">
      <p class="muted">
        旧的恢复密钥已失效。新的密钥只显示这一次，请妥善保存。
      </p>
      <div class="codes">
        <code v-for="c in codes" :key="c">{{ c }}</code>
      </div>
      <template #footer>
        <el-button @click="copy(codes.join('\n'))">复制全部</el-button>
        <el-button type="primary" @click="showCodes = false">已保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  addUser,
  deleteUser,
  listUsers,
  regenerateRecovery,
  resetLocal2FA,
  setUserNote,
  userAction,
} from '../api/auth'
import type { AdminUser, UserList, UserStatus } from '../api/auth'
import { useAuth } from '../composables/useAuth'

const { session } = useAuth()
const data = ref<UserList | null>(null)
const newUser = ref('')
const codes = ref<string[]>([])
const showCodes = ref(false)
let timer: number | null = null

const pending = computed(
  () => data.value?.users.filter((u) => u.status === 'pending').length || 0,
)

const statusLabel = (s: UserStatus) =>
  ({
    pending: '待批准',
    approved: '已批准',
    rejected: '已拒绝',
    disabled: '已停用',
  })[s] || s
const statusTag = (s: UserStatus) =>
  (
    ({
      pending: 'warning',
      approved: 'success',
      rejected: 'danger',
      disabled: 'info',
    }) as const
  )[s]

const fmt = (t?: string) =>
  !t || t.startsWith('0001') ? '—' : new Date(t).toLocaleString()

const fetchData = async () => {
  try {
    data.value = await listUsers()
  } catch (e: any) {
    ElMessage.error('加载用户失败：' + e.message)
  }
}

const copy = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制')
  } catch {
    ElMessage.info(text)
  }
}

const onAdd = async () => {
  const name = newUser.value.trim()
  if (!name) return
  try {
    await addUser(name, '')
    newUser.value = ''
    ElMessage.success(`已登记 ${name}，对方可以直接通过 SSO 登录`)
    await fetchData()
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

const labels = {
  approve: '批准',
  reject: '拒绝',
  disable: '停用',
  enable: '启用',
}

const act = async (row: AdminUser, action: keyof typeof labels) => {
  if (action !== 'approve' && action !== 'enable') {
    try {
      await ElMessageBox.confirm(
        `确定${labels[action]}用户 ${row.username}？`,
        labels[action],
        {
          type: 'warning',
          confirmButtonText: labels[action],
          cancelButtonText: '取消',
        },
      )
    } catch {
      return
    }
  }
  try {
    await userAction(row.username, action)
    ElMessage.success(`已${labels[action]}`)
    await fetchData()
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

const onDelete = async (row: AdminUser) => {
  try {
    await ElMessageBox.confirm(
      `确定删除用户 ${row.username}？删除后对方再次登录需要重新批准。`,
      '删除',
      {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消',
      },
    )
  } catch {
    return
  }
  try {
    await deleteUser(row.username)
    await fetchData()
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

const onNote = async (row: AdminUser) => {
  try {
    const { value } = await ElMessageBox.prompt(
      '备注',
      `编辑 ${row.username} 的备注`,
      {
        inputValue: row.note || '',
        confirmButtonText: '保存',
        cancelButtonText: '取消',
      },
    )
    await setUserNote(row.username, value || '')
    await fetchData()
  } catch (e: any) {
    if (e !== 'cancel' && e !== 'close')
      ElMessage.error(e?.message || String(e))
  }
}

const onRegenerate = async () => {
  try {
    const res = await regenerateRecovery()
    codes.value = res.recoveryCodes
    showCodes.value = true
    await fetchData()
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

const onReset = async () => {
  try {
    await ElMessageBox.confirm(
      '重置后本地账号的动态验证码和恢复密钥全部失效，本地账号会被登出，下次登录时需要重新绑定。确定重置？',
      '重置二次验证',
      { type: 'warning', confirmButtonText: '重置', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    await resetLocal2FA()
    ElMessage.success('已重置')
    await fetchData()
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

onMounted(() => {
  fetchData()
  timer = window.setInterval(fetchData, 5000)
})
onUnmounted(() => {
  if (timer !== null) window.clearInterval(timer)
})
</script>

<style scoped>
.ent-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 20px;
  flex-wrap: wrap;
}
.title-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.page-title {
  font-size: 28px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin: 0;
  line-height: 1.2;
}
.page-subtitle {
  font-size: 14px;
  color: var(--el-text-color-secondary);
  margin: 0;
}
.header-actions {
  display: flex;
  gap: 8px;
}
.name-input {
  width: 220px;
}
.panel {
  border-radius: 8px;
}
.count {
  margin-left: 8px;
}
.local {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}
.local-name,
.name {
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.local-actions {
  display: flex;
  gap: 8px;
}
.muted {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}
.note {
  cursor: pointer;
  border-bottom: 1px dashed var(--el-border-color);
}
.codes {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  padding: 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}
.codes code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  text-align: center;
}
@media (max-width: 768px) {
  .header-actions {
    width: 100%;
  }
  .name-input {
    flex: 1;
    width: auto;
  }
}
</style>
