import { ref } from 'vue'

// Recent samples of the overview's numbers, kept while the dashboard is
// open, for the cards' dot-matrix sparklines.
export interface Sample {
  at: number
  clients: number
  proxies: number
  conns: number
  traffic: number
  rate: number
  ports: number
  codes: number
}

const MAX = 120
const samples = ref<Sample[]>([])

export const useSamples = () => {
  const push = (s: Omit<Sample, 'rate' | 'at'>) => {
    const now = Date.now()
    const prev = samples.value[samples.value.length - 1]
    let rate = 0
    if (prev && s.traffic >= prev.traffic && now > prev.at) {
      rate = ((s.traffic - prev.traffic) * 1000) / (now - prev.at)
    }
    samples.value = [...samples.value, { ...s, at: now, rate }].slice(-MAX)
  }
  const series = (key: keyof Sample) => samples.value.map((s) => s[key])
  return { samples, push, series }
}
