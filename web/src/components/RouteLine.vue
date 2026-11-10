<script setup lang="ts">
import { computed } from 'vue'
import type { ExitKind } from '../lib/proxy'

const props = defineProps<{
  kind: ExitKind
  label: string
  /** Компактный вариант без подписи режима. */
  compact?: boolean
}>()

const modeText = computed(() => {
  switch (props.kind) {
    case 'unknown':
      return 'Неизвестно'
    case 'custom':
      return 'Свой прокси'
    case 'default':
      return 'Общий прокси'
    default:
      return 'Без прокси'
  }
})
</script>

<template>
  <span class="route" :class="`route--${kind}`">
    <span class="route__wire" aria-hidden="true">
      <span class="route__line" />
      <span class="route__node" />
    </span>
    <span class="route__text">
      <span v-if="!compact" class="route__mode">{{ modeText }}</span>
      <span v-else class="visually-hidden">{{ modeText }}: </span>
      <span class="route__exit" :class="{ mono: kind !== 'direct' }" :title="label">{{ label }}</span>
    </span>
  </span>
</template>

<style scoped>
.route {
  --c: var(--route-direct);
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
  max-width: 100%;
}
.route--unknown {
  --c: var(--ink-3);
}
.route--custom {
  --c: var(--route-custom);
}
.route--default {
  --c: var(--route-default);
}
.route__wire {
  display: inline-flex;
  align-items: center;
  flex: none;
  width: 40px;
}
.route__line {
  flex: 1;
  height: 0;
  border-top: 2px solid var(--c);
}
.route--default .route__line {
  border-top-style: dashed;
}
.route--direct .route__line {
  border-top-style: dotted;
  border-top-width: 3px;
}
.route__node {
  width: 9px;
  height: 9px;
  border: 2px solid var(--c);
  border-radius: 50%;
  background: var(--c);
}
.route--direct .route__node {
  background: transparent;
}
.route__text {
  display: grid;
  min-width: 0;
  line-height: 1.3;
}
.route__mode {
  font-size: var(--fs-xs);
  color: var(--c);
  font-weight: 600;
}
.route__exit {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--fs-sm);
}
</style>
