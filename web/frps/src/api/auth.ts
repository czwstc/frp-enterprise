import { buildQueryString, http } from './http'

export interface AuthConfig {
  enabled: boolean
  oidc?: boolean
  oidcName?: string
}

export interface SessionInfo {
  username: string
  source: 'local' | 'oidc'
  createdAt: string
  expiresAt: string
}

export interface LoginResult {
  status: 'ok' | 'totp_required' | 'enroll'
  enrollToken?: string
  secret?: string
  otpauthURL?: string
  recoveryCodes?: string[]
  session?: SessionInfo
  recoveryUsed?: boolean
  recoveryLeft?: number
}

export interface LocalAccount {
  username: string
  totpEnabled: boolean
  enrolledAt?: string
  recoveryLeft: number
  lastLoginAt?: string
  lastLoginIP?: string
}

export type UserStatus = 'pending' | 'approved' | 'rejected' | 'disabled'

export interface AdminUser {
  username: string
  status: UserStatus
  email?: string
  note?: string
  requestedAt?: string
  createdAt: string
  decidedBy?: string
  decidedAt?: string
  lastLoginAt?: string
  lastLoginIP?: string
}

export interface UserList {
  local: LocalAccount
  oidcEnabled: boolean
  users: AdminUser[]
}

export interface AuditEntry {
  time: string
  actor: string
  action: string
  target?: string
  detail?: string
  ip?: string
  success: boolean
}

export interface RuntimeLogLine {
  seq: number
  time: number
  level: string
  text: string
}

export const getAuthConfig = () => http.getV2<AuthConfig>('../api/auth/config')
export const getMe = () => http.getV2<SessionInfo>('../api/auth/me')
export const login = (username: string, password: string, code = '') =>
  http.postV2<LoginResult>('../api/auth/login', { username, password, code })
export const confirmEnroll = (enrollToken: string, code: string) =>
  http.postV2<LoginResult>('../api/auth/enroll', { enrollToken, code })
export const logout = () => http.postV2<null>('../api/auth/logout')

export const listUsers = () => http.getV2<UserList>('../api/admin/users')
export const addUser = (username: string, note: string) =>
  http.postV2<AdminUser>('../api/admin/users', { username, note })
export const userAction = (
  username: string,
  action: 'approve' | 'reject' | 'disable' | 'enable',
) =>
  http.postV2<AdminUser>(
    `../api/admin/users/${encodeURIComponent(username)}/${action}`,
  )
export const setUserNote = (username: string, note: string) =>
  http.postV2<AdminUser>(
    `../api/admin/users/${encodeURIComponent(username)}/note`,
    { note },
  )
export const deleteUser = (username: string) =>
  http.deleteV2<unknown>(`../api/admin/users/${encodeURIComponent(username)}`)
export const regenerateRecovery = () =>
  http.postV2<{ recoveryCodes: string[] }>('../api/admin/local/recovery-codes')
export const resetLocal2FA = () =>
  http.postV2<unknown>('../api/admin/local/reset-2fa')

export const listAudit = (params: {
  q?: string
  action?: string
  offset?: number
  limit?: number
}) =>
  http.getV2<{ total: number; items: AuditEntry[] }>(
    `../api/admin/audit${buildQueryString({
      q: params.q || undefined,
      action: params.action || undefined,
      offset: params.offset,
      limit: params.limit,
    })}`,
  )

export const listRuntimeLogs = (params: { level?: string; q?: string }) =>
  http.getV2<RuntimeLogLine[]>(
    `../api/admin/logs${buildQueryString({
      level: params.level || undefined,
      q: params.q || undefined,
      limit: 1000,
    })}`,
  )
