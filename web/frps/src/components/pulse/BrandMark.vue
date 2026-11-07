<template>
  <div class="brand-mark">
    <img
      v-if="branding.logo"
      :src="branding.logo"
      :alt="branding.siteName"
      class="logo-img"
    />
    <!-- Placeholder for the Huiding wordmark until a logo is uploaded -->
    <span v-else class="placeholder" aria-hidden="true">
      <i>hJ</i>
      <span class="ph-text"><b>汇鼎科技</b><small>HUIDING GROUP</small></span>
    </span>
    <div
      v-if="showName && (branding.logoMode === 'both' || !branding.logo)"
      class="names"
    >
      <div class="name">{{ branding.siteName }}</div>
      <div v-if="subtitle" class="sub">{{ subtitle }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
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
.logo-img {
  max-height: 30px;
  max-width: 160px;
  display: block;
}
.placeholder {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: none;
}
.placeholder i {
  font-family: Georgia, 'Times New Roman', serif;
  font-weight: 700;
  font-size: 22px;
  color: var(--accent);
  line-height: 1;
}
.ph-text {
  display: grid;
  line-height: 1.1;
}
.ph-text b {
  font-size: 12px;
  color: var(--el-text-color-primary);
}
.ph-text small {
  font-size: 7px;
  letter-spacing: 0.1em;
  color: var(--el-text-color-secondary);
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
