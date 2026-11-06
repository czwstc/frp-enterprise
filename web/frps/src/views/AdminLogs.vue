<template>
  <div class="ent-page">
    <div class="title-section">
      <h1 class="page-title">日志</h1>
      <p class="page-subtitle">
        审计日志记录谁在什么时候做了什么；运行日志是 frps 最近的输出。
      </p>
    </div>

    <el-tabs v-model="tab">
      <el-tab-pane label="审计日志" name="audit">
        <div class="filters">
          <el-select v-model="action" class="action-select" @change="reload">
            <el-option
              v-for="a in actions"
              :key="a.value"
              :label="a.label"
              :value="a.value"
            />
          </el-select>
          <el-input
            v-model="q"
            placeholder="搜索用户、鉴权码、IP…"
            clearable
            class="search"
            @input="debouncedReload"
          />
        </div>
        <el-table
          v-loading="loading"
          :data="audit"
          empty-text="暂无记录"
          size="small"
        >
          <el-table-column label="时间" width="170">
            <template #default="{ row }">{{
              new Date(row.time).toLocaleString()
            }}</template>
          </el-table-column>
          <el-table-column prop="actor" label="操作人" width="130" />
          <el-table-column label="操作" width="200">
            <template #default="{ row }">
              <el-tag
                :type="row.success ? 'info' : 'danger'"
                size="small"
                disable-transitions
              >
                {{ actionLabel(row.action) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="target" label="对象" min-width="160" />
          <el-table-column prop="detail" label="详情" min-width="200" />
          <el-table-column prop="ip" label="IP" width="130" />
        </el-table>
        <el-pagination
          v-if="total > pageSize"
          class="pager"
          layout="total, prev, pager, next"
          :total="total"
          :page-size="pageSize"
          :current-page="page"
          @current-change="onPage"
        />
      </el-tab-pane>

      <el-tab-pane label="运行日志" name="runtime">
        <div class="filters">
          <el-radio-group v-model="level" @change="loadRuntime">
            <el-radio-button value="">全部</el-radio-button>
            <el-radio-button value="info">信息</el-radio-button>
            <el-radio-button value="warn">警告</el-radio-button>
            <el-radio-button value="error">错误</el-radio-button>
          </el-radio-group>
          <el-input
            v-model="rq"
            placeholder="搜索…"
            clearable
            class="search"
            @input="loadRuntime"
          />
          <el-switch v-model="follow" active-text="自动刷新" />
        </div>
        <div ref="logBox" class="log-box">
          <div v-if="runtime.length === 0" class="muted">暂无日志</div>
          <div
            v-for="l in runtime"
            :key="l.seq"
            class="log-line"
            :class="l.level"
          >
            <span class="time">{{
              new Date(l.time).toLocaleTimeString()
            }}</span>
            <span class="text">{{ l.text }}</span>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { listAudit, listRuntimeLogs } from '../api/auth'
import type { AuditEntry, RuntimeLogLine } from '../api/auth'

const tab = ref('audit')
const audit = ref<AuditEntry[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const q = ref('')
const action = ref('')
const loading = ref(false)

const runtime = ref<RuntimeLogLine[]>([])
const level = ref('')
const rq = ref('')
const follow = ref(true)
const logBox = ref<HTMLElement | null>(null)
let timer: number | null = null
let debounce: number | null = null

const actions = [
  { value: '', label: '全部操作' },
  { value: 'auth.', label: '登录 / 登出' },
  { value: 'user.', label: '用户管理' },
  { value: 'code.', label: '鉴权码' },
  { value: 'notice.', label: '通知' },
  { value: 'client.', label: '客户端连接' },
]

const actionLabels: Record<string, string> = {
  'auth.local.login': '本地登录',
  'auth.local.enroll_start': '开始设置二次验证',
  'auth.local.enroll': '设置二次验证',
  'auth.local.reset_2fa': '重置二次验证',
  'auth.local.recovery_regenerate': '重新生成恢复密钥',
  'auth.oidc.login': 'SSO 登录',
  'auth.logout': '登出',
  'user.request': '申请登录',
  'user.add': '登记用户',
  'user.approved': '批准用户',
  'user.rejected': '拒绝用户',
  'user.disabled': '停用用户',
  'user.delete': '删除用户',
  'user.note': '修改用户备注',
  'code.generate': '生成鉴权码',
  'code.approve': '批准鉴权码',
  'code.reject': '拒绝鉴权码',
  'code.revoke': '吊销鉴权码',
  'code.unbind': '解除设备绑定',
  'code.note': '修改鉴权码备注',
  'code.delete': '删除鉴权码',
  'notice.send': '发送通知',
  'client.login': '客户端登录被拒',
  'client.connect': '客户端连接',
  'client.disconnect': '客户端断开',
}
const actionLabel = (a: string) => actionLabels[a] || a

const reload = async () => {
  loading.value = true
  try {
    const res = await listAudit({
      q: q.value,
      action: action.value,
      offset: (page.value - 1) * pageSize,
      limit: pageSize,
    })
    audit.value = res.items
    total.value = res.total
  } catch (e: any) {
    ElMessage.error('加载审计日志失败：' + e.message)
  } finally {
    loading.value = false
  }
}

const debouncedReload = () => {
  if (debounce !== null) window.clearTimeout(debounce)
  debounce = window.setTimeout(() => {
    page.value = 1
    reload()
  }, 300)
}

const onPage = (p: number) => {
  page.value = p
  reload()
}

const loadRuntime = async () => {
  try {
    const box = logBox.value
    const atEnd =
      !box || box.scrollTop + box.clientHeight >= box.scrollHeight - 20
    runtime.value = await listRuntimeLogs({ level: level.value, q: rq.value })
    if (atEnd) {
      await nextTick()
      if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight
    }
  } catch (e: any) {
    ElMessage.error('加载运行日志失败：' + e.message)
  }
}

watch(tab, (t) => (t === 'runtime' ? loadRuntime() : reload()))

onMounted(() => {
  reload()
  timer = window.setInterval(() => {
    if (tab.value === 'runtime' && follow.value) loadRuntime()
    else if (tab.value === 'audit' && page.value === 1) reload()
  }, 3000)
})
onUnmounted(() => {
  if (timer !== null) window.clearInterval(timer)
  if (debounce !== null) window.clearTimeout(debounce)
})
</script>

<style scoped>
.ent-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
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
.filters {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.action-select {
  width: 160px;
}
.search {
  width: 260px;
}
.pager {
  margin-top: 12px;
  justify-content: flex-end;
}
.log-box {
  height: 60vh;
  overflow: auto;
  padding: 12px;
  border-radius: 8px;
  border: 1px solid var(--el-border-color-lighter);
  background: var(--el-bg-color);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.6;
}
.log-line {
  display: flex;
  gap: 10px;
  color: var(--el-text-color-regular);
}
.log-line.warn {
  color: var(--el-color-warning);
}
.log-line.error {
  color: var(--el-color-danger);
}
.log-line.debug,
.log-line.trace {
  color: var(--el-text-color-secondary);
}
.time {
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
}
.text {
  white-space: pre-wrap;
  word-break: break-all;
}
.muted {
  color: var(--el-text-color-secondary);
}
</style>
