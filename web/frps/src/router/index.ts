import { createRouter, createWebHashHistory } from 'vue-router'
import ServerOverview from '../views/ServerOverview.vue'
import Clients from '../views/Clients.vue'
import ClientDetail from '../views/ClientDetail.vue'
import Proxies from '../views/Proxies.vue'
import ProxyDetail from '../views/ProxyDetail.vue'
import AuthCodes from '../views/AuthCodes.vue'
import ServerNotices from '../views/ServerNotices.vue'
import LoginPage from '../views/LoginPage.vue'
import AdminUsers from '../views/AdminUsers.vue'
import AdminLogs from '../views/AdminLogs.vue'
import { useAuth } from '../composables/useAuth'

const router = createRouter({
  history: createWebHashHistory(),
  scrollBehavior() {
    return { top: 0 }
  },
  routes: [
    {
      path: '/',
      name: 'ServerOverview',
      component: ServerOverview,
    },
    {
      path: '/clients',
      name: 'Clients',
      component: Clients,
    },
    {
      path: '/clients/:key',
      name: 'ClientDetail',
      component: ClientDetail,
    },
    {
      path: '/proxies/:type?',
      name: 'Proxies',
      component: Proxies,
    },
    {
      path: '/proxy/:name',
      name: 'ProxyDetail',
      component: ProxyDetail,
    },
    {
      path: '/auth-codes',
      name: 'AuthCodes',
      component: AuthCodes,
    },
    {
      path: '/notices',
      name: 'Notices',
      component: ServerNotices,
    },
    {
      path: '/login',
      name: 'Login',
      component: LoginPage,
      meta: { public: true },
    },
    {
      path: '/users',
      name: 'Users',
      component: AdminUsers,
      meta: { accounts: true },
    },
    {
      path: '/logs',
      name: 'Logs',
      component: AdminLogs,
      meta: { accounts: true },
    },
  ],
})

// With dashboard accounts, every page but the login page needs a session.
router.beforeEach(async (to) => {
  if (to.meta.public) return true
  const { refresh, config } = useAuth()
  if (!(await refresh())) {
    return { path: '/login' }
  }
  if (to.meta.accounts && !config.value?.enabled) {
    return { path: '/' }
  }
  return true
})

export default router
