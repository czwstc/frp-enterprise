import { ref } from 'vue'
import { getBranding } from '../api/branding'
import type { Branding } from '../api/branding'

// The built-in Huiding look, used until the server answers.
export const defaultBranding = (): Branding => ({
  siteName: 'HDIT FRPE',
  subtitle: '内网穿透控制台',
  logoMode: 'both',
  theme: 'system',
  accent: '#1f5eff',
  topLink: {
    mode: 'custom',
    url: 'https://www.huiding-group.com/',
    text: '汇鼎科技官网',
  },
  version: 0,
})

const branding = ref<Branding>(defaultBranding())
const loaded = ref(false)
let media: MediaQueryList | null = null

const hexToRgb = (hex: string) => {
  const v = parseInt(hex.slice(1), 16)
  return [(v >> 16) & 255, (v >> 8) & 255, v & 255]
}

// mix blends color a toward b by t (0..1).
const mix = (a: string, b: string, t: number) => {
  const x = hexToRgb(a)
  const y = hexToRgb(b)
  const c = x.map((v, i) => Math.round(v + (y[i] - v) * t))
  return '#' + c.map((v) => v.toString(16).padStart(2, '0')).join('')
}

const isDark = () => document.documentElement.classList.contains('dark')

// applyAccent maps the accent onto Element Plus's primary color scale.
const applyAccent = () => {
  const accent = /^#[0-9a-f]{6}$/i.test(branding.value.accent)
    ? branding.value.accent
    : '#1f5eff'
  const dark = isDark()
  const base = dark ? mix(accent, '#ffffff', 0.18) : accent
  const bg = dark ? '#141414' : '#ffffff'
  const s = document.documentElement.style
  s.setProperty('--accent', base)
  s.setProperty('--el-color-primary', base)
  for (const [n, t] of [
    [3, 0.3],
    [5, 0.5],
    [7, 0.7],
    [8, 0.8],
    [9, 0.9],
  ] as const) {
    s.setProperty(`--el-color-primary-light-${n}`, mix(base, bg, t))
  }
  s.setProperty('--el-color-primary-dark-2', mix(base, '#000000', 0.2))
  s.setProperty('--accent-soft', mix(base, bg, dark ? 0.78 : 0.88))
}

const applyTheme = () => {
  const theme = branding.value.theme
  const dark =
    theme === 'dark' ||
    (theme === 'system' &&
      window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
  applyAccent()
}

export const applyBranding = (b: Branding) => {
  branding.value = b
  document.title = b.siteName
  applyTheme()
}

export const useBranding = () => {
  const load = async () => {
    if (!media) {
      media = window.matchMedia('(prefers-color-scheme: dark)')
      media.addEventListener('change', applyTheme)
    }
    try {
      applyBranding(await getBranding())
    } catch {
      applyBranding(defaultBranding())
    }
    loaded.value = true
  }
  return { branding, loaded, load, applyBranding }
}
