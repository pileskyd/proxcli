<script setup lang="ts">
import { computed } from 'vue'
import { providerColor, providerKey, providerLabel } from '../lib/providers'

const props = defineProps<{
  /** Сырое имя провайдера или канонический ключ. */
  provider: string
  size?: number
}>()

const key = computed(() => providerKey(props.provider))
const color = computed(() => providerColor(key.value))
const initial = computed(() => providerLabel(key.value).slice(0, 1).toUpperCase())
</script>

<template>
  <svg
    class="glyph"
    viewBox="0 0 20 20"
    :width="size ?? 18"
    :height="size ?? 18"
    :style="{ color }"
    aria-hidden="true"
    focusable="false"
  >
    <!-- Claude: лучистая звезда. -->
    <g v-if="key === 'claude'" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
      <path d="M10 2.2v15.6M2.2 10h15.6M4.5 4.5l11 11M15.5 4.5l-11 11" />
      <path d="M6.9 2.9l6.2 14.2M2.9 13.1l14.2-6.2M2.9 6.9l14.2 6.2M13.1 2.9L6.9 17.1" stroke-width="1.3" />
    </g>
    <!-- Codex: скруглённый квадрат с приглашением терминала. -->
    <g v-else-if="key === 'codex'">
      <rect x="1.5" y="1.5" width="17" height="17" rx="4.5" fill="currentColor" />
      <path d="M6 7.2l2.8 2.8L6 12.8M10.6 13h3.6" fill="none" stroke="#0e0e0e" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" />
    </g>
    <!-- Gemini и родственные: четырёхлучевая звезда. -->
    <path
      v-else-if="key === 'gemini' || key === 'gemini-cli' || key === 'antigravity'"
      d="M10 1.5c.7 4.6 3.9 7.8 8.5 8.5-4.6.7-7.8 3.9-8.5 8.5-.7-4.6-3.9-7.8-8.5-8.5 4.6-.7 7.8-3.9 8.5-8.5z"
      fill="currentColor"
    />
    <!-- Провайдер не указан: пунктирный круг. -->
    <g v-else-if="key === 'unknown'" fill="none" stroke="currentColor" stroke-width="1.6">
      <circle cx="10" cy="10" r="7.5" stroke-dasharray="2.6 2.4" />
      <path d="M8.2 8.2a1.9 1.9 0 0 1 3.6.7c0 1.2-1.8 1.5-1.8 2.6" stroke-linecap="round" />
      <path d="M10 13.9h.01" stroke-width="2" stroke-linecap="round" />
    </g>
    <!-- Остальные: кружок с первой буквой. -->
    <g v-else>
      <circle cx="10" cy="10" r="8.5" fill="currentColor" opacity="0.18" />
      <text x="10" y="13.6" text-anchor="middle" font-size="10" font-weight="650" fill="currentColor" font-family="inherit">{{ initial }}</text>
    </g>
  </svg>
</template>

<style scoped>
.glyph {
  flex: none;
  display: block;
}
</style>
