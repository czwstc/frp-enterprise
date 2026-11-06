export type AuthCodeStatus = 'pending' | 'approved' | 'rejected' | 'revoked'

export interface AuthCode {
  code: string
  status: AuthCodeStatus
  source: 'server' | 'client'
  note?: string
  boundClientID?: string
  hostname?: string
  os?: string
  arch?: string
  version?: string
  lastIP?: string
  createdAt: string
  updatedAt: string
  lastSeenAt?: string
  online: boolean
  runID?: string
}

export interface EnterpriseStatus {
  enabled: boolean
  pending: number
}

export interface Notice {
  id: string
  level: 'info' | 'warning' | 'error' | 'success'
  title?: string
  content?: string
  target?: string
  time: string
}

export interface PortInUse {
  proto: string
  port: number
  proxyName: string
  runID: string
  authCode?: string
}

export type CodeAction = 'approve' | 'reject' | 'revoke' | 'unbind'
