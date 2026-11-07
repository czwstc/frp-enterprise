<template>
  <div class="overview">
    <p class="intro">
      {{ uptimeText
      }}<span v-if="samples.length > 1">
        · 曲线为最近 {{ windowText }} 的采样</span
      >
    </p>

    <div class="cards">
      <MetricCard
        title="客户端"
        icon="monitor"
        color="--pulse-blue"
        caption="在线"
        :value="info?.status.clientCounts ?? '—'"
        unit="台"
        :stats="[
          { label: '待审批', value: codeStats.pending },
          { label: '已绑定设备', value: codeStats.bound },
          { label: '已吊销', value: codeStats.blocked },
        ]"
        :series="series('clients')"
        to="/clients"
      />
      <MetricCard
        title="端口映射"
        icon="link"
        color="--pulse-green"
        caption="运行中"
        :value="proxyTotal"
        unit="个"
        :tag="proxyTotal > 0 ? '正常' : undefined"
        tag-type="ok"
        :stats="[
          { label: 'TCP', value: proxyCount('tcp') },
          { label: 'UDP', value: proxyCount('udp') },
          {
            label: 'HTTP(S) 等',
            value: proxyTotal - proxyCount('tcp') - proxyCount('udp'),
          },
        ]"
        :series="series('proxies')"
        to="/proxies"
      />
      <MetricCard
        title="流量"
        icon="swap"
        color="--pulse-pink"
        caption="今日合计"
        :value="split(trafficTotal)[0]"
        :unit="split(trafficTotal)[1]"
        :stats="[
          {
            label: '入站',
            value: formatFileSize(info?.status.totalTrafficIn ?? 0),
          },
          {
            label: '出站',
            value: formatFileSize(info?.status.totalTrafficOut ?? 0),
          },
          { label: '当前速率', value: formatFileSize(currentRate) + '/s' },
        ]"
        :series="series('rate')"
      />
      <MetricCard
        title="连接"
        icon="bolt"
        color="--pulse-orange"
        caption="当前"
        :value="info?.status.curConns ?? '—'"
        unit="条"
        :stats="[
          { label: '峰值', value: peak('conns') },
          { label: '平均', value: average('conns') },
          { label: '每台客户端', value: perClient },
        ]"
        :series="series('conns')"
      />
      <MetricCard
        title="端口占用"
        icon="grid"
        color="--pulse-teal"
        :caption="
          allowed
            ? `已用 / 允许范围 ${info?.config.allowPortsStr}`
            : '已用（未限制范围）'
        "
        :value="ports.length"
        :unit="allowed ? `/ ${allowed}` : '个'"
        :stats="[
          { label: '剩余', value: allowed ? allowed - ports.length : '不限' },
          {
            label: 'TCP',
            value: ports.filter((p) => p.proto === 'tcp').length,
          },
          {
            label: 'UDP',
            value: ports.filter((p) => p.proto === 'udp').length,
          },
        ]"
        :series="series('ports')"
        :max="allowed || 0"
        to="/notices"
      />
      <MetricCard
        title="鉴权码"
        icon="key"
        color="--pulse-violet"
        caption="已批准"
        :value="entEnabled ? codeStats.approved : '未开启'"
        :unit="entEnabled ? '个' : ''"
        :tag="codeStats.pending ? `${codeStats.pending} 个待审批` : undefined"
        tag-type="warn"
        :stats="[
          { label: '服务端生成', value: codeStats.server },
          { label: '客户端申请', value: codeStats.client },
          { label: '未绑定', value: codeStats.unbound },
        ]"
        :series="series('codes')"
        to="/auth-codes"
      />
    </div>

    <div class="split">
      <section class="panel">
        <header>
          <SideIcon name="bell" color="var(--pulse-amber)" />
          待处理
          <span class="muted">{{ pendingItems.length }}</span>
        </header>
        <div v-if="pendingItems.length === 0" class="empty">
          没有需要处理的申请。
        </div>
        <div v-for="p in pendingItems" :key="p.key" class="row">
          <div class="row-main">
            <div class="t1">
              {{ p.kind === 'code' ? '鉴权码' : 'SSO 用户' }}
              <span :class="{ mono: p.kind === 'code' }">{{ p.name }}</span>
            </div>
            <div class="t2">{{ p.detail }}</div>
          </div>
          <div class="btns">
            <el-button size="small" type="success" @click="decide(p, true)"
              >批准</el-button
            >
            <el-button
              size="small"
              type="danger"
              plain
              @click="decide(p, false)"
              >拒绝</el-button
            >
          </div>
        </div>
      </section>

      <section class="panel">
        <header>
          <SideIcon name="list" color="var(--pulse-gray)" />
          最近审计
          <router-link v-if="authEnabled" to="/logs" class="more"
            >全部 ›</router-link
          >
        </header>
        <div v-if="!authEnabled" class="empty">
          开启控制台账号（enterprise.admin）后记录审计日志。
        </div>
        <div v-else-if="audit.length === 0" class="empty">暂无记录。</div>
        <div v-for="(a, i) in audit" :key="i" class="row audit">
          <time>{{ hhmm(a.time) }}</time>
          <div class="row-main">
            <div class="t1">
              {{ a.actor }} {{ actionLabel(a.action) }}
              <span v-if="!a.success" class="bad">失败</span>
            </div>
            <div class="t2">
              {{ [a.target, a.detail, a.ip].filter(Boolean).join(' · ') }}
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import MetricCard from '../components/pulse/MetricCard.vue'
import SideIcon from '../components/pulse/SideIcon.vue'
import { getServerInfo } from '../api/server'
import {
  authCodeAction,
  getEnterpriseStatus,
  listAuthCodes,
  listPortsInUse,
} from '../api/enterprise'
import { listAudit, listUsers, userAction } from '../api/auth'
import type { AuditEntry, AdminUser } from '../api/auth'
import type { AuthCode, PortInUse } from '../types/enterprise'
import type { ServerInfo } from '../types/server'
import { useAuth } from '../composables/useAuth'
import { useSamples } from '../composables/useSamples'
import { formatFileSize } from '../utils/format'

const { config: authConfig } = useAuth()
const { samples, push, series } = useSamples()

const info = ref<ServerInfo | null>(null)
const codes = ref<AuthCode[]>([])
const ports = ref<PortInUse[]>([])
const users = ref<AdminUser[]>([])
const audit = ref<AuditEntry[]>([])
const entEnabled = ref(false)
let timer: number | null = null

const authEnabled = computed(() => !!authConfig.value?.enabled)

const proxyCount = (t: string) => info.value?.status.proxyTypeCount?.[t] ?? 0
const proxyTotal = computed(() =>
  Object.values(info.value?.status.proxyTypeCount || {}).reduce(
    (a, b) => a + b,
    0,
  ),
)
const trafficTotal = computed(
  () =>
    (info.value?.status.totalTrafficIn ?? 0) +
    (info.value?.status.totalTrafficOut ?? 0),
)
const currentRate = computed(
  () => samples.value[samples.value.length - 1]?.rate ?? 0,
)

const split = (bytes: number) => {
  const [v, u] = formatFileSize(bytes).split(' ')
  return [v, u || '']
}

// allowed is the number of ports in the server's allowPorts ranges.
const allowed = computed(() => {
  const s = info.value?.config.allowPortsStr || ''
  if (!s) return 0
  return s.split(',').reduce((n, part) => {
    const [a, b] = part.trim().split('-').map(Number)
    if (!a) return n
    return n + (b ? b - a + 1 : 1)
  }, 0)
})

const codeStats = computed(() => {
  const c = codes.value
  return {
    approved: c.filter((x) => x.status === 'approved').length,
    pending: c.filter((x) => x.status === 'pending').length,
    blocked: c.filter((x) => x.status === 'revoked' || x.status === 'rejected')
      .length,
    bound: c.filter((x) => x.boundClientID && x.status === 'approved').length,
    unbound: c.filter((x) => !x.boundClientID && x.status === 'approved')
      .length,
    server: c.filter((x) => x.source === 'server').length,
    client: c.filter((x) => x.source === 'client').length,
  }
})

const peak = (k: 'conns') =>
  samples.value.length ? Math.max(...series(k)) : '—'
const average = (k: 'conns') => {
  const s = series(k)
  return s.length ? Math.round(s.reduce((a, b) => a + b, 0) / s.length) : '—'
}
const perClient = computed(() => {
  const c = info.value?.status.clientCounts || 0
  return c ? Math.round((info.value?.status.curConns || 0) / c) : 0
})

const windowText = computed(() => {
  const ms = Date.now() - (samples.value[0]?.at ?? Date.now())
  const min = Math.max(1, Math.round(ms / 60000))
  return `${min} 分钟`
})
const uptimeText = computed(() => {
  const v = info.value?.version
  return v ? `frps ${v}` : '正在读取服务端状态…'
})

interface Pending {
  key: string
  kind: 'code' | 'user'
  name: string
  detail: string
}

const ago = (t?: string) => {
  if (!t || t.startsWith('0001')) return ''
  const m = Math.round((Date.now() - new Date(t).getTime()) / 60000)
  if (m < 1) return '刚刚'
  if (m < 60) return `${m} 分钟前`
  const h = Math.round(m / 60)
  return h < 48 ? `${h} 小时前` : new Date(t).toLocaleDateString()
}

const pendingItems = computed<Pending[]>(() => [
  ...codes.value
    .filter((c) => c.status === 'pending')
    .map((c) => ({
      key: 'c:' + c.code,
      kind: 'code' as const,
      name: c.code,
      detail: [
        '客户端申请',
        c.hostname,
        c.lastIP,
        ago(c.lastSeenAt || c.createdAt),
      ]
        .filter(Boolean)
        .join(' · '),
    })),
  ...users.value
    .filter((u) => u.status === 'pending')
    .map((u) => ({
      key: 'u:' + u.username,
      kind: 'user' as const,
      name: u.username,
      detail: ['首次通过 SSO 登录，等待批准', u.email, ago(u.requestedAt)]
        .filter(Boolean)
        .join(' · '),
    })),
])

const decide = async (p: Pending, approve: boolean) => {
  try {
    if (p.kind === 'code')
      await authCodeAction(p.name, approve ? 'approve' : 'reject')
    else await userAction(p.name, approve ? 'approve' : 'reject')
    ElMessage.success(approve ? '已批准' : '已拒绝')
    await refresh()
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

const actionLabels: Record<string, string> = {
  'auth.local.login': '本地登录',
  'auth.local.enroll': '设置二次验证',
  'auth.local.enroll_start': '开始设置二次验证',
  'auth.local.reset_2fa': '重置二次验证',
  'auth.local.recovery_regenerate': '重新生成恢复密钥',
  'auth.oidc.login': '通过 SSO 登录',
  'auth.logout': '登出',
  'user.request': '申请登录',
  'user.add': '登记用户',
  'user.approved': '批准用户',
  'user.rejected': '拒绝用户',
  'user.disabled': '停用用户',
  'user.delete': '删除用户',
  'code.generate': '生成鉴权码',
  'code.approve': '批准鉴权码',
  'code.reject': '拒绝鉴权码',
  'code.revoke': '吊销鉴权码',
  'code.unbind': '解除设备绑定',
  'code.delete': '删除鉴权码',
  'notice.send': '发送通知',
  'settings.branding': '修改外观设置',
  'client.login': '客户端登录被拒',
  'client.connect': '客户端连接',
  'client.disconnect': '客户端断开',
}
const actionLabel = (a: string) => actionLabels[a] || a
const hhmm = (t: string) => {
  const d = new Date(t)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

const refresh = async () => {
  try {
    info.value = await getServerInfo()
  } catch {
    // Shown as dashes.
  }
  try {
    const st = await getEnterpriseStatus()
    entEnabled.value = st.enabled
    if (st.enabled) {
      ;[codes.value, ports.value] = await Promise.all([
        listAuthCodes(),
        listPortsInUse(),
      ])
    }
  } catch {
    entEnabled.value = false
  }
  if (authEnabled.value) {
    try {
      const [u, a] = await Promise.all([listUsers(), listAudit({ limit: 6 })])
      users.value = u.users
      audit.value = a.items
    } catch {
      // Optional panels.
    }
  }
  if (info.value) {
    push({
      clients: info.value.status.clientCounts,
      proxies: proxyTotal.value,
      conns: info.value.status.curConns,
      traffic: trafficTotal.value,
      ports: ports.value.length,
      codes: codeStats.value.approved,
    })
  }
}

onMounted(() => {
  refresh()
  timer = window.setInterval(refresh, 5000)
})
onUnmounted(() => {
  if (timer !== null) window.clearInterval(timer)
})
</script>

<style scoped>
.overview {
  display: grid;
  gap: 14px;
}
.intro {
  margin: 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.cards {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}
.split {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr);
  gap: 12px;
}
.panel {
  background: var(--pulse-card);
  border: 1px solid var(--pulse-line);
  border-radius: 12px;
  padding: 12px 14px;
  box-shadow: var(--pulse-shadow);
  min-width: 0;
}
.panel header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 12.5px;
  margin-bottom: 4px;
}
.panel .muted,
.panel .more {
  margin-left: auto;
  font-weight: 500;
  color: var(--el-text-color-secondary);
  text-decoration: none;
  font-size: 12px;
}
.row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  padding: 8px 0;
  border-top: 1px solid var(--pulse-line);
}
.row.audit {
  grid-template-columns: 52px minmax(0, 1fr);
}
.row:first-of-type {
  border-top: 0;
}
.row-main {
  min-width: 0;
}
.t1 {
  font-weight: 550;
  font-size: 13px;
}
.t2 {
  font-size: 11.5px;
  color: var(--el-text-color-secondary);
  margin-top: 2px;
  overflow-wrap: anywhere;
}
.mono,
time {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
}
time {
  color: var(--el-text-color-secondary);
}
.btns {
  display: flex;
  gap: 4px;
}
.bad {
  color: var(--pulse-red);
  font-size: 11px;
  margin-left: 4px;
}
.empty {
  color: var(--el-text-color-secondary);
  font-size: 12.5px;
  padding: 10px 0;
}
@media (max-width: 1100px) {
  .cards {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .split {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 640px) {
  .cards {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
