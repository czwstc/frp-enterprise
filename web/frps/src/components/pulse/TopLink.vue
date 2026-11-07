<template>
  <a
    v-if="link.mode === 'github'"
    class="top-link icon-only"
    href="https://github.com/czwstc/frp-enterprise"
    target="_blank"
    rel="noopener"
    aria-label="GitHub"
    title="GitHub"
  >
    <GitHubIcon class="gh" />
  </a>
  <a
    v-else-if="link.mode === 'custom' && safeUrl"
    class="top-link"
    :href="safeUrl"
    target="_blank"
    rel="noopener"
    :title="link.text"
  >
    <img v-if="link.image" :src="link.image" alt="" />
    <svg
      v-else
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      stroke-width="1.8"
      stroke-linecap="round"
    >
      <path d="M6 3H3v10h10v-3M9 2h5v5M14 2L7 9" />
    </svg>
    <span v-if="link.text">{{ link.text }}</span>
  </a>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import GitHubIcon from '../../assets/icons/github.svg?component'
import type { TopLink } from '../../api/branding'

const props = defineProps<{ link: TopLink }>()
const safeUrl = computed(() =>
  /^https?:\/\//i.test(props.link.url || '') ? props.link.url : '',
)
</script>

<style scoped>
.top-link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 10px;
  border-radius: 8px;
  border: 1px solid var(--pulse-line);
  background: var(--pulse-card);
  color: var(--el-text-color-primary);
  font-size: 12.5px;
  text-decoration: none;
}
.top-link:hover {
  border-color: var(--accent);
  color: var(--accent);
}
.top-link img {
  width: 18px;
  height: 18px;
  object-fit: contain;
  border-radius: 4px;
}
.top-link svg {
  width: 13px;
  height: 13px;
}
.icon-only {
  padding: 0 7px;
}
.gh {
  width: 16px;
  height: 16px;
}
</style>
