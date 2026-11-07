import { http } from './http'

export interface TopLink {
  mode: 'custom' | 'github' | 'none'
  url?: string
  text?: string
  image?: string
}

export interface Branding {
  siteName: string
  subtitle: string
  logo?: string
  logoMode: 'both' | 'logo'
  logoBackdrop: 'halo' | 'plate' | 'none'
  theme: 'system' | 'light' | 'dark'
  accent: string
  topLink: TopLink
  version: number
  updatedBy?: string
  updatedAt?: string
}

export const getBranding = () => http.getV2<Branding>('../api/branding')

export const saveBranding = (b: Branding) =>
  http.putV2<Branding>('../api/branding', b)
