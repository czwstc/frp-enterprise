<template>
  <div v-if="route.meta.public" id="app" class="bare">
    <router-view></router-view>
  </div>
  <div v-else id="app" class="shell">
    <div
      v-if="isMobile && sidebarOpen"
      class="overlay"
      @click="sidebarOpen = false"
    />

    <aside class="side" :class="{ open: !isMobile || sidebarOpen }">
      <router-link to="/" class="brand" @click="closeSidebar">
        <BrandMark />
      </router-link>
      <nav class="groups">
        <div v-for="group in nav" :key="group.label" class="group">
          <div class="group-label">{{ group.label }}</div>
          <router-link
            v-for="item in group.items"
            :key="item.to"
            :to="item.to"
            class="item"
            :class="{ on: isActive(item) }"
            @click="closeSidebar"
          >
            <SideIcon :name="item.icon" :color="`var(${item.color})`" />
            <span class="label">{{ item.label }}</span>
            <span v-if="item.alert" class="alert">{{ item.alert }}</span>
            <span v-else-if="item.count !== undefined" class="count">{{
              item.count
            }}</span>
          </router-link>
        </div>
      </nav>
      <div class="side-foot">
        <div>{{ versionText }}</div>
        <div v-if="authConfig?.enabled">审计日志已开启</div>
      </div>
    </aside>

    <main id="content">
      <header class="topbar">
        <button
          v-if="isMobile"
          class="hamburger"
          aria-label="打开菜单"
          @click="sidebarOpen = !sidebarOpen"
        >
          <svg
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            stroke-linecap="round"
          >
            <path d="M2 4h12M2 8h12M2 12h12" />
          </svg>
        </button>
        <div class="title-block">
          <div class="page-name">{{ pageTitle }}</div>
        </div>
        <div class="spacer"></div>
        <TopLink :link="branding.topLink" />
        <el-dropdown v-if="session" trigger="click" @command="onUserCommand">
          <span class="user-chip">
            <span class="avatar">{{
              session.username.slice(0, 1).toUpperCase()
            }}</span>
            {{ session.username }}
            <small>{{ session.source === 'local' ? '本地' : 'SSO' }}</small>
          </span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="logout">退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </header>
      <div class="page">
        <router-view></router-view>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import BrandMark from './components/pulse/BrandMark.vue'
import SideIcon from './components/pulse/SideIcon.vue'
import TopLink from './components/pulse/TopLink.vue'
import { useResponsive } from './composables/useResponsive'
import { useBranding } from './composables/useBranding'
import { useAuth } from './composables/useAuth'
import { getEnterpriseStatus } from './api/enterprise'
import { getServerInfo } from './api/server'
import { listUsers, logout } from './api/auth'
import { setUnauthorizedHandler } from './api/http'

interface NavItem {
  to: string
  label: string
  icon: string
  color: string
  count?: number
  alert?: number
  match?: string[]
}

const route = useRoute()
const router = useRouter()
const { isMobile } = useResponsive()
const { branding, load: loadBranding } = useBranding()
const { config: authConfig, session } = useAuth()

const sidebarOpen = ref(false)
const closeSidebar = () => {
  if (isMobile.value) sidebarOpen.value = false
}

const clients = ref<number | undefined>()
const proxies = ref<number | undefined>()
const pendingCodes = ref(0)
const pendingUsers = ref(0)
const version = ref('')

const nav = computed<{ label: string; items: NavItem[] }[]>(() => {
  const groups: { label: string; items: NavItem[] }[] = [
    {
      label: '监控',
      items: [
        { to: '/', label: '概览', icon: 'home', color: '--pulse-blue' },
        {
          to: '/clients',
          label: '客户端',
          icon: 'monitor',
          color: '--pulse-blue',
          count: clients.value,
        },
        {
          to: '/proxies',
          label: '代理',
          icon: 'link',
          color: '--pulse-green',
          count: proxies.value,
          match: ['/proxies', '/proxy'],
        },
      ],
    },
    {
      label: '管理',
      items: [
        {
          to: '/auth-codes',
          label: '鉴权码',
          icon: 'key',
          color: '--pulse-violet',
          alert: pendingCodes.value || undefined,
        },
        { to: '/notices', label: '通知', icon: 'bell', color: '--pulse-amber' },
        ...(authConfig.value?.enabled
          ? [
              {
                to: '/users',
                label: '用户',
                icon: 'users',
                color: '--pulse-teal',
                alert: pendingUsers.value || undefined,
              },
            ]
          : []),
      ],
    },
    {
      label: '系统',
      items: [
        ...(authConfig.value?.enabled
          ? [
              {
                to: '/logs',
                label: '日志',
                icon: 'list',
                color: '--pulse-orange',
              },
            ]
          : []),
        {
          to: '/server',
          label: '服务端信息',
          icon: 'server',
          color: '--pulse-pink',
        },
        {
          to: '/preferences',
          label: '设置',
          icon: 'gear',
          color: '--pulse-gray',
        },
      ],
    },
  ]
  return groups
})

const isActive = (item: NavItem) => {
  if (item.to === '/') return route.path === '/'
  return (item.match || [item.to]).some((p) => route.path.startsWith(p))
}

const pageTitle = computed(
  () => (route.meta.title as string) || branding.value.siteName,
)
const versionText = computed(() =>
  version.value ? `frps ${version.value} · 企业版` : 'frps 企业版',
)

// A request answered 401 means the session has ended: go to the login page.
setUnauthorizedHandler(() => {
  if (authConfig.value?.enabled && !route.meta.public) {
    session.value = null
    router.replace('/login')
  }
})

const onUserCommand = async (cmd: string) => {
  if (cmd !== 'logout') return
  try {
    await logout()
  } catch {
    // The session is gone either way.
  }
  session.value = null
  router.replace('/login')
}

let timer: number | null = null
const refreshCounts = async () => {
  if (route.meta.public) return
  try {
    const info = await getServerInfo()
    version.value = info.version
    clients.value = info.status.clientCounts
    proxies.value = Object.values(info.status.proxyTypeCount || {}).reduce(
      (a, b) => a + b,
      0,
    )
  } catch {
    // Counts are optional.
  }
  try {
    const status = await getEnterpriseStatus()
    pendingCodes.value = status.enabled ? status.pending : 0
  } catch {
    pendingCodes.value = 0
  }
  if (authConfig.value?.enabled) {
    try {
      const users = await listUsers()
      pendingUsers.value = users.users.filter(
        (u) => u.status === 'pending',
      ).length
    } catch {
      pendingUsers.value = 0
    }
  }
}

watch(
  () => route.path,
  () => {
    closeSidebar()
    refreshCounts()
  },
)

onMounted(() => {
  loadBranding()
  refreshCounts()
  timer = window.setInterval(refreshCounts, 5000)
})
onUnmounted(() => {
  if (timer !== null) window.clearInterval(timer)
})
</script>

<style>
html,
body {
  height: 100%;
  margin: 0;
  overflow: hidden;
}
*,
:after,
:before {
  box-sizing: border-box;
  -webkit-tap-highlight-color: transparent;
}
#app {
  height: 100vh;
  height: 100dvh;
  background: var(--pulse-canvas);
  color: var(--el-text-color-primary);
}
#app.bare {
  overflow: auto;
}
</style>

<style scoped>
.shell {
  display: grid;
  grid-template-columns: 228px minmax(0, 1fr);
}
.side {
  background: var(--pulse-side);
  border-right: 1px solid var(--pulse-line);
  padding: 12px 10px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  overflow-y: auto;
  min-height: 0;
}
.brand {
  display: block;
  padding: 6px 6px 4px;
  text-decoration: none;
  min-height: 36px;
}
.groups {
  display: grid;
  gap: 14px;
}
.group {
  display: grid;
  gap: 1px;
}
.group-label {
  font-size: 11px;
  color: var(--el-text-color-placeholder);
  font-weight: 600;
  padding: 4px 8px;
  letter-spacing: 0.03em;
}
.item {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 6px 8px;
  border-radius: 7px;
  color: var(--el-text-color-primary);
  text-decoration: none;
  font-size: 13px;
}
.item:hover {
  background: var(--pulse-side-hover);
}
.item.on {
  background: var(--accent);
  color: #fff;
}
.item.on .count {
  color: rgba(255, 255, 255, 0.85);
}
.label {
  flex: 1;
  min-width: 0;
}
.count {
  font-size: 11.5px;
  color: var(--el-text-color-secondary);
  font-variant-numeric: tabular-nums;
}
.alert {
  background: var(--pulse-red);
  color: #fff;
  border-radius: 9px;
  padding: 0 6px;
  font-size: 11px;
  line-height: 17px;
  font-weight: 600;
}
.side-foot {
  margin-top: auto;
  padding: 8px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
  border-top: 1px solid var(--pulse-line);
}
#content {
  min-width: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}
.topbar {
  position: sticky;
  top: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 24px;
  background: color-mix(in srgb, var(--pulse-canvas) 88%, transparent);
  backdrop-filter: blur(10px);
}
.page-name {
  font-size: 17px;
  font-weight: 650;
}
.spacer {
  flex: 1;
}
.user-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px 4px 4px;
  border-radius: 16px;
  background: var(--pulse-side);
  font-size: 12.5px;
  color: var(--el-text-color-primary);
  cursor: pointer;
}
.user-chip small {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}
.avatar {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--accent);
  color: #fff;
  display: grid;
  place-items: center;
  font-size: 11px;
  font-weight: 600;
}
.page {
  padding: 4px 24px 32px;
  min-width: 0;
}
.hamburger {
  border: 0;
  background: transparent;
  color: var(--el-text-color-primary);
  width: 32px;
  height: 32px;
  display: grid;
  place-items: center;
  cursor: pointer;
}
.hamburger svg {
  width: 18px;
  height: 18px;
}
.overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.35);
  z-index: 19;
}
@media (max-width: 767px) {
  .shell {
    grid-template-columns: minmax(0, 1fr);
  }
  .side {
    position: fixed;
    inset: 0 auto 0 0;
    width: 240px;
    z-index: 20;
    transform: translateX(-100%);
    transition: transform 0.2s;
  }
  .side.open {
    transform: none;
  }
  .topbar,
  .page {
    padding-left: 16px;
    padding-right: 16px;
  }
}
</style>
