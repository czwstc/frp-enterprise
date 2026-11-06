<template>
  <div class="ent-page">
    <div class="page-header">
      <div class="title-section">
        <h1 class="page-title">鉴权码</h1>
        <p class="page-subtitle">
          生成鉴权码发给客户端，或审批客户端自己生成的鉴权码。每个鉴权码只能绑定一台设备。
        </p>
      </div>
      <div class="header-actions">
        <el-input
          v-model="newNote"
          placeholder="备注（可选），例如：张三的笔记本"
          class="note-input"
          clearable
          @keyup.enter="onGenerate"
        />
        <el-button type="primary" :loading="generating" @click="onGenerate">
          生成鉴权码
        </el-button>
      </div>
    </div>

    <el-alert
      v-if="!enabled && loaded"
      type="warning"
      :closable="false"
      title="服务端未开启鉴权码登录"
      description="在 frps 配置中设置 enterprise.enabled = true 后重启 frps。"
    />

    <el-alert
      v-if="lastGenerated"
      type="success"
      :closable="true"
      @close="lastGenerated = ''"
    >
      <template #title>
        已生成鉴权码
        <code class="code">{{ lastGenerated }}</code>
        <el-button link type="primary" @click="copy(lastGenerated)"
          >复制</el-button
        >
      </template>
    </el-alert>

    <div class="status-tabs">
      <button
        v-for="tab in tabs"
        :key="tab.value"
        class="status-tab"
        :class="{ active: statusFilter === tab.value }"
        @click="statusFilter = tab.value"
      >
        <span class="tab-label">{{ tab.label }}</span>
        <span class="tab-count">{{ tab.count }}</span>
      </button>
    </div>

    <el-table
      v-loading="loading"
      :data="filtered"
      class="codes-table"
      empty-text="暂无鉴权码"
    >
      <el-table-column label="鉴权码" min-width="190">
        <template #default="{ row }">
          <div class="code-cell">
            <code class="code">{{ row.code }}</code>
            <el-button link size="small" @click="copy(row.code)"
              >复制</el-button
            >
          </div>
          <div class="muted">
            {{ row.source === 'server' ? '服务端生成' : '客户端提交' }}
          </div>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tag :type="statusTag(row.status)" disable-transitions>
            {{ statusLabel(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="在线" width="80">
        <template #default="{ row }">
          <span class="dot" :class="row.online ? 'online' : 'offline'"></span>
          {{ row.online ? '在线' : '离线' }}
        </template>
      </el-table-column>
      <el-table-column label="设备" min-width="200">
        <template #default="{ row }">
          <div v-if="row.hostname || row.boundClientID">
            <div>{{ row.hostname || '-' }}</div>
            <div class="muted">
              {{ [row.os, row.arch].filter(Boolean).join('/') }}
              <span v-if="row.boundClientID">· {{ row.boundClientID }}</span>
            </div>
          </div>
          <span v-else class="muted">未绑定</span>
        </template>
      </el-table-column>
      <el-table-column label="IP / 最后在线" min-width="170">
        <template #default="{ row }">
          <div>{{ row.lastIP || '-' }}</div>
          <div class="muted">{{ formatTime(row.lastSeenAt) }}</div>
        </template>
      </el-table-column>
      <el-table-column label="备注" min-width="140">
        <template #default="{ row }">
          <span class="note" @click="editNote(row)">{{ row.note || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <el-button
            v-if="row.status !== 'approved'"
            size="small"
            type="success"
            @click="act(row, 'approve')"
          >
            批准
          </el-button>
          <el-button
            v-if="row.status === 'pending'"
            size="small"
            type="danger"
            @click="act(row, 'reject')"
          >
            拒绝
          </el-button>
          <el-button
            v-if="row.status === 'approved'"
            size="small"
            type="danger"
            @click="act(row, 'revoke')"
          >
            吊销
          </el-button>
          <el-dropdown
            trigger="click"
            @command="(cmd: string) => onMore(row, cmd)"
          >
            <el-button size="small">更多</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item
                  command="unbind"
                  :disabled="!row.boundClientID"
                >
                  解除设备绑定
                </el-dropdown-item>
                <el-dropdown-item command="note">编辑备注</el-dropdown-item>
                <el-dropdown-item command="delete" divided
                  >删除</el-dropdown-item
                >
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  authCodeAction,
  deleteAuthCode,
  generateAuthCode,
  getEnterpriseStatus,
  listAuthCodes,
  setAuthCodeNote,
} from '../api/enterprise'
import type { AuthCode, AuthCodeStatus, CodeAction } from '../types/enterprise'

type Filter = 'all' | 'pending' | 'approved' | 'blocked'

const codes = ref<AuthCode[]>([])
const loading = ref(false)
const loaded = ref(false)
const enabled = ref(true)
const generating = ref(false)
const newNote = ref('')
const lastGenerated = ref('')
const statusFilter = ref<Filter>('pending')
let timer: number | null = null

const count = (pred: (c: AuthCode) => boolean) =>
  codes.value.filter(pred).length

const tabs = computed(() => [
  {
    value: 'pending' as Filter,
    label: '待审批',
    count: count((c) => c.status === 'pending'),
  },
  {
    value: 'approved' as Filter,
    label: '已批准',
    count: count((c) => c.status === 'approved'),
  },
  {
    value: 'blocked' as Filter,
    label: '已拒绝 / 吊销',
    count: count((c) => c.status === 'rejected' || c.status === 'revoked'),
  },
  { value: 'all' as Filter, label: '全部', count: codes.value.length },
])

const filtered = computed(() =>
  codes.value.filter((c) => {
    switch (statusFilter.value) {
      case 'pending':
        return c.status === 'pending'
      case 'approved':
        return c.status === 'approved'
      case 'blocked':
        return c.status === 'rejected' || c.status === 'revoked'
      default:
        return true
    }
  }),
)

const statusLabel = (s: AuthCodeStatus) =>
  ({
    pending: '待审批',
    approved: '已批准',
    rejected: '已拒绝',
    revoked: '已吊销',
  })[s] || s

const statusTag = (s: AuthCodeStatus) =>
  (
    ({
      pending: 'warning',
      approved: 'success',
      rejected: 'danger',
      revoked: 'info',
    }) as const
  )[s]

const formatTime = (t?: string) => {
  if (!t || t.startsWith('0001')) return '从未连接'
  return new Date(t).toLocaleString()
}

const fetchData = async (silent = false) => {
  if (!silent) loading.value = true
  try {
    const status = await getEnterpriseStatus()
    enabled.value = status.enabled
    codes.value = status.enabled ? await listAuthCodes() : []
  } catch (e: any) {
    if (!silent) ElMessage.error('加载鉴权码失败：' + e.message)
  } finally {
    loading.value = false
    loaded.value = true
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

const onGenerate = async () => {
  generating.value = true
  try {
    const c = await generateAuthCode(newNote.value)
    lastGenerated.value = c.code
    newNote.value = ''
    statusFilter.value = 'approved'
    await fetchData(true)
  } catch (e: any) {
    ElMessage.error('生成失败：' + e.message)
  } finally {
    generating.value = false
  }
}

const actionLabels: Record<CodeAction, string> = {
  approve: '批准',
  reject: '拒绝',
  revoke: '吊销',
  unbind: '解除绑定',
}

const act = async (row: AuthCode, action: CodeAction) => {
  if (action === 'revoke' || action === 'reject' || action === 'unbind') {
    try {
      await ElMessageBox.confirm(
        `确定${actionLabels[action]}鉴权码 ${row.code}？${row.online ? '该客户端会被立即断开。' : ''}`,
        actionLabels[action],
        {
          type: 'warning',
          confirmButtonText: actionLabels[action],
          cancelButtonText: '取消',
        },
      )
    } catch {
      return
    }
  }
  try {
    await authCodeAction(row.code, action)
    ElMessage.success(`已${actionLabels[action]}`)
    await fetchData(true)
  } catch (e: any) {
    ElMessage.error(`${actionLabels[action]}失败：` + e.message)
  }
}

const editNote = async (row: AuthCode) => {
  try {
    const { value } = await ElMessageBox.prompt(
      '备注',
      `编辑 ${row.code} 的备注`,
      {
        inputValue: row.note || '',
        confirmButtonText: '保存',
        cancelButtonText: '取消',
      },
    )
    await setAuthCodeNote(row.code, value || '')
    await fetchData(true)
  } catch (e: any) {
    if (e !== 'cancel' && e !== 'close')
      ElMessage.error('保存失败：' + (e?.message || e))
  }
}

const onMore = async (row: AuthCode, cmd: string) => {
  if (cmd === 'unbind') return act(row, 'unbind')
  if (cmd === 'note') return editNote(row)
  if (cmd === 'delete') {
    try {
      await ElMessageBox.confirm(`确定删除鉴权码 ${row.code}？`, '删除', {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消',
      })
    } catch {
      return
    }
    try {
      await deleteAuthCode(row.code)
      await fetchData(true)
    } catch (e: any) {
      ElMessage.error('删除失败：' + e.message)
    }
  }
}

onMounted(() => {
  fetchData()
  timer = window.setInterval(() => fetchData(true), 3000)
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

.note-input {
  width: 260px;
}

.status-tabs {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.status-tab {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  border: 1px solid var(--el-border-color);
  border-radius: 20px;
  background: var(--el-bg-color);
  color: var(--el-text-color-regular);
  font-size: 14px;
  cursor: pointer;
}

.status-tab.active {
  border-color: var(--el-color-primary);
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.tab-count {
  font-weight: 600;
}

.codes-table {
  border-radius: 8px;
}

.code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
  font-weight: 600;
}

.code-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.muted {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.note {
  cursor: pointer;
  border-bottom: 1px dashed var(--el-border-color);
}

.dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 4px;
}

.dot.online {
  background: var(--el-color-success);
}

.dot.offline {
  background: var(--el-text-color-placeholder);
}

@media (max-width: 768px) {
  .header-actions {
    width: 100%;
  }
  .note-input {
    flex: 1;
    width: auto;
  }
}
</style>
