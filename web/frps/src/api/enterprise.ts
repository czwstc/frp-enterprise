import { buildQueryString, http } from './http'
import type {
  AuthCode,
  CodeAction,
  EnterpriseStatus,
  Notice,
  PortInUse,
} from '../types/enterprise'

export const getEnterpriseStatus = () =>
  http.getV2<EnterpriseStatus>('../api/enterprise/status')

export const listAuthCodes = (status?: string) =>
  http.getV2<AuthCode[]>(
    `../api/enterprise/codes${buildQueryString({ status: status || undefined })}`,
  )

export const generateAuthCode = (note: string) =>
  http.postV2<AuthCode>('../api/enterprise/codes', { note })

export const authCodeAction = (code: string, action: CodeAction) =>
  http.postV2<AuthCode>(
    `../api/enterprise/codes/${encodeURIComponent(code)}/${action}`,
  )

export const setAuthCodeNote = (code: string, note: string) =>
  http.postV2<AuthCode>(
    `../api/enterprise/codes/${encodeURIComponent(code)}/note`,
    { note },
  )

export const deleteAuthCode = (code: string) =>
  http.deleteV2<{ code: string }>(
    `../api/enterprise/codes/${encodeURIComponent(code)}`,
  )

export const listNotices = () =>
  http.getV2<Notice[]>('../api/enterprise/notices')

export const sendNotice = (notice: {
  level: string
  title: string
  content: string
  target: string
}) => http.postV2<Notice>('../api/enterprise/notices', notice)

export const listPortsInUse = () =>
  http.getV2<PortInUse[]>('../api/enterprise/ports')
