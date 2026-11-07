<template>
  <article
    class="metric-card"
    :class="{ link: !!to }"
    @click="to && $router.push(to)"
  >
    <header>
      <SideIcon :name="icon" :color="`var(${color})`" />
      <span class="title">{{ title }}</span>
      <span v-if="to" class="go">›</span>
    </header>
    <div>
      <div class="k">{{ caption }}</div>
      <div class="big">
        {{ value }}<small v-if="unit">{{ unit }}</small>
        <span v-if="tag" class="tag" :class="tagType">{{ tag }}</span>
      </div>
    </div>
    <div v-if="stats?.length" class="trio">
      <div v-for="s in stats" :key="s.label">
        <span class="k">{{ s.label }}</span>
        <b>{{ s.value }}</b>
      </div>
    </div>
    <DotSpark
      :values="series || []"
      :color="color"
      :max="max"
      :label="`${title}趋势`"
    />
  </article>
</template>

<script setup lang="ts">
import SideIcon from './SideIcon.vue'
import DotSpark from './DotSpark.vue'

defineProps<{
  title: string
  icon: string
  color: string
  caption: string
  value: string | number
  unit?: string
  tag?: string
  tagType?: 'ok' | 'warn' | 'bad'
  stats?: { label: string; value: string | number }[]
  series?: number[]
  max?: number
  to?: string
}>()
</script>

<style scoped>
.metric-card {
  background: var(--pulse-card);
  border: 1px solid var(--pulse-line);
  border-radius: 12px;
  padding: 12px 14px;
  display: grid;
  gap: 8px;
  min-width: 0;
  box-shadow: var(--pulse-shadow);
}
.metric-card.link {
  cursor: pointer;
}
.metric-card.link:hover {
  border-color: color-mix(in srgb, var(--accent) 45%, var(--pulse-line));
}
header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 12.5px;
  color: var(--el-text-color-primary);
}
.go {
  margin-left: auto;
  color: var(--el-text-color-placeholder);
}
.k {
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.big {
  font-size: 26px;
  font-weight: 650;
  letter-spacing: -0.01em;
  font-variant-numeric: tabular-nums;
  line-height: 1.15;
  color: var(--el-text-color-primary);
}
.big small {
  font-size: 13px;
  font-weight: 500;
  color: var(--el-text-color-secondary);
  margin-left: 3px;
}
.trio {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 6px;
}
.trio b {
  display: block;
  font-size: 12.5px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--el-text-color-primary);
}
.tag {
  font-size: 10.5px;
  padding: 1px 7px;
  border-radius: 10px;
  font-weight: 600;
  margin-left: 6px;
  vertical-align: 4px;
}
.tag.ok {
  background: color-mix(in srgb, var(--pulse-green) 16%, transparent);
  color: var(--pulse-green);
}
.tag.warn {
  background: color-mix(in srgb, var(--pulse-amber) 18%, transparent);
  color: var(--pulse-amber);
}
.tag.bad {
  background: color-mix(in srgb, var(--pulse-red) 16%, transparent);
  color: var(--pulse-red);
}
</style>
