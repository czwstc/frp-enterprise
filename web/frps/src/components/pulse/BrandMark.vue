<template>
  <div class="brand-mark">
    <span class="logo-wrap" :class="`bd-${branding.logoBackdrop || 'halo'}`">
      <img
        :src="branding.logo || defaultLogo"
        :alt="branding.siteName"
        class="logo-img"
      />
    </span>
    <div
      v-if="showName && branding.logoMode === 'both'"
      class="names with-logo"
    >
      <div class="name">{{ branding.siteName }}</div>
      <div v-if="subtitle" class="sub">{{ subtitle }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import defaultLogo from '../../assets/huiding-logo.png'
import { useBranding } from '../../composables/useBranding'

const props = withDefaults(
  defineProps<{ showName?: boolean; sub?: string }>(),
  {
    showName: true,
    sub: undefined,
  },
)
const { branding } = useBranding()
const subtitle = computed(() => props.sub ?? branding.value.subtitle)
</script>

<style scoped>
.brand-mark {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.logo-wrap {
  display: inline-flex;
  flex: none;
}
.logo-img {
  height: 30px;
  max-width: 190px;
  object-fit: contain;
  display: block;
}
/* A dark glow traced around the logo's own shape keeps a white logo
   readable on the light sidebar. */
.bd-halo .logo-img {
  filter: drop-shadow(0 0 1px rgba(0, 0, 0, 0.85))
    drop-shadow(0 0 2px rgba(0, 0, 0, 0.6))
    drop-shadow(0 1px 4px rgba(0, 0, 0, 0.35));
}
.bd-plate {
  background: #1b2233;
  border-radius: 8px;
  padding: 6px 10px;
}
.brand-mark:has(.with-logo) .logo-img {
  height: 22px;
  max-width: 96px;
}
.names {
  min-width: 0;
  border-left: 1px solid var(--pulse-line);
  padding-left: 10px;
}
.name {
  font-weight: 650;
  font-size: 13px;
  color: var(--el-text-color-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sub {
  font-size: 11px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
</style>
