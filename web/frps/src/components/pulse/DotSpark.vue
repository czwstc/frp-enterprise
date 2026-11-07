<template>
  <canvas ref="cv" class="dot-spark" :aria-label="label" role="img"></canvas>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

// A dot-matrix sparkline: one column of dots per value, lit up to its level.
const props = withDefaults(
  defineProps<{
    values: number[]
    color: string
    label?: string
    max?: number
  }>(),
  { label: '', max: 0 },
)
const cv = ref<HTMLCanvasElement | null>(null)
let ro: ResizeObserver | null = null

const draw = () => {
  const el = cv.value
  if (!el) return
  const dpr = window.devicePixelRatio || 1
  const w = el.clientWidth
  const h = el.clientHeight
  if (!w || !h) return
  el.width = w * dpr
  el.height = h * dpr
  const ctx = el.getContext('2d')
  if (!ctx) return
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, w, h)
  const style = getComputedStyle(el)
  const on = style.getPropertyValue(props.color).trim() || props.color
  const off = style.getPropertyValue('--pulse-dot-off').trim() || '#e7eaf0'
  const step = 4
  const cols = Math.floor(w / step)
  const rows = Math.floor(h / step)
  const vals = props.values.slice(-cols)
  const max = props.max || Math.max(1, ...vals)
  const offset = cols - vals.length
  for (let x = 0; x < cols; x++) {
    const v = x < offset ? 0 : vals[x - offset]
    const lit = v > 0 ? Math.max(1, Math.round((v / max) * rows)) : 0
    for (let y = 0; y < rows; y++) {
      const level = rows - y
      const isOn = level <= lit
      ctx.globalAlpha = isOn ? 0.45 + 0.55 * (level / rows) : 1
      ctx.fillStyle = isOn ? on : off
      ctx.fillRect(x * step, y * step, 2.4, 2.4)
    }
  }
  ctx.globalAlpha = 1
}

watch(() => [props.values, props.color, props.max], draw, { deep: true })
onMounted(() => {
  draw()
  ro = new ResizeObserver(draw)
  if (cv.value) ro.observe(cv.value)
  const mo = new MutationObserver(draw)
  mo.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class', 'style'],
  })
  onBeforeUnmount(() => mo.disconnect())
})
onBeforeUnmount(() => ro?.disconnect())
</script>

<style scoped>
.dot-spark {
  width: 100%;
  height: 34px;
  display: block;
}
</style>
