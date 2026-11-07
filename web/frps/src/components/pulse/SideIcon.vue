<template>
  <span class="side-icon" :style="{ background: color }" aria-hidden="true">
    <svg
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      stroke-width="1.8"
      stroke-linecap="round"
      stroke-linejoin="round"
      v-html="paths[name] || ''"
    ></svg>
  </span>
</template>

<script setup lang="ts">
defineProps<{ name: string; color: string }>()

// Small strokes drawn in white on a colored square, as in a source list.
const paths: Record<string, string> = {
  home: '<path d="M2 8l6-5 6 5v6H2z"/>',
  monitor: '<path d="M2 3h12v8H2zM6 14h4"/>',
  link: '<path d="M6.5 9.5l3-3M5 11a2.5 2.5 0 010-3.5L7 5.5M11 5a2.5 2.5 0 010 3.5L9 10.5"/>',
  swap: '<path d="M3 5h9l-2-2M13 11H4l2 2"/>',
  bolt: '<path d="M9 1L3 9h4l-1 6 6-8H8z"/>',
  grid: '<path d="M2 2h5v5H2zM9 2h5v5H9zM2 9h5v5H2zM9 9h5v5H9z"/>',
  key: '<circle cx="5" cy="8" r="3"/><path d="M8 8h6M12 8v3"/>',
  bell: '<path d="M4 11V7a4 4 0 018 0v4l1 1H3zM7 14h2"/>',
  list: '<path d="M3 4h10M3 8h10M3 12h6"/>',
  users:
    '<circle cx="6" cy="6" r="2.5"/><path d="M1.5 14a4.5 4.5 0 019 0M11 4a2.3 2.3 0 010 4.5M12.5 14a4 4 0 00-2-3.4"/>',
  gear: '<circle cx="8" cy="8" r="2.2"/><path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M3.4 12.6l1.4-1.4M11.2 4.8l1.4-1.4"/>',
  server: '<path d="M2 2h12v5H2zM2 9h12v5H2zM5 4.5h.01M5 11.5h.01"/>',
}
</script>

<style scoped>
.side-icon {
  width: 18px;
  height: 18px;
  border-radius: 5px;
  display: inline-grid;
  place-items: center;
  color: #fff;
  flex: none;
}
.side-icon svg {
  width: 11px;
  height: 11px;
}
</style>
