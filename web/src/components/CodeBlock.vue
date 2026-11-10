<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  code: string
  /** Путь к файлу или назначение блока, показывается в шапке. */
  title: string
  /** Что копировать, если отличается от показанного (например, ключ скрыт на экране). */
  copyText?: string
}>()

const copied = ref<'idle' | 'done' | 'failed'>('idle')
let timer = 0

async function copy() {
  window.clearTimeout(timer)
  try {
    await navigator.clipboard.writeText(props.copyText ?? props.code)
    copied.value = 'done'
  } catch {
    copied.value = 'failed'
  }
  timer = window.setTimeout(() => (copied.value = 'idle'), 2400)
}
</script>

<template>
  <figure class="code">
    <figcaption class="code__head">
      <span class="code__title mono">{{ title }}</span>
      <button type="button" class="btn btn--quiet btn--small" @click="copy">
        {{ copied === 'done' ? 'Скопировано' : copied === 'failed' ? 'Не удалось скопировать' : 'Копировать' }}
      </button>
      <span class="visually-hidden" aria-live="polite">{{ copied === 'done' ? 'Скопировано в буфер обмена' : '' }}</span>
    </figcaption>
    <pre class="code__body" tabindex="0" :aria-label="`Содержимое: ${title}`"><code>{{ code }}</code></pre>
  </figure>
</template>

<style scoped>
.code {
  margin: 0;
  border: 1px solid var(--rule);
  border-radius: var(--r-panel);
  background: var(--surface);
  overflow: hidden;
  min-width: 0;
}
.code__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: 6px var(--space-2) 6px var(--space-4);
  border-bottom: 1px solid var(--rule);
}
.code__title {
  font-size: var(--fs-sm);
  color: var(--ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.code__body {
  margin: 0;
  padding: var(--space-4);
  overflow-x: auto;
  font-size: var(--fs-sm);
  line-height: 1.6;
  tab-size: 2;
  color: #e5e5e5;
}
.code__body:focus-visible {
  outline-offset: -2px;
}
</style>
