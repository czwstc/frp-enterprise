import { ref } from 'vue'
import { getAuthConfig, getMe } from '../api/auth'
import type { AuthConfig, SessionInfo } from '../api/auth'

// Shared sign-in state of the dashboard.
const config = ref<AuthConfig | null>(null)
const session = ref<SessionInfo | null>(null)

export const useAuth = () => {
  const loadConfig = async () => {
    if (!config.value) {
      try {
        config.value = await getAuthConfig()
      } catch {
        config.value = { enabled: false }
      }
    }
    return config.value
  }

  // refresh reports whether the dashboard may be shown.
  const refresh = async (): Promise<boolean> => {
    const cfg = await loadConfig()
    if (!cfg.enabled) return true
    try {
      session.value = await getMe()
      return true
    } catch {
      session.value = null
      return false
    }
  }

  return { config, session, loadConfig, refresh }
}
