<script setup lang="ts">
import AppIcon from './AppIcon.vue'
import ProviderGlyph from './ProviderGlyph.vue'
import { providerLabel } from '../lib/providers'

defineProps<{
  /** Канонические ключи провайдеров и число подписок у каждого. */
  items: readonly { key: string; count: number }[]
  total: number
  label?: string
}>()

const model = defineModel<string>({ required: true })
</script>

<template>
  <div class="tabs" role="group" :aria-label="label ?? 'Провайдер'">
    <button type="button" class="tab" :aria-pressed="model === 'all'" @click="model = 'all'">
      <AppIcon name="all" class="tab__all" />
      <span>Все</span>
      <span class="tab__count">{{ total }}</span>
    </button>
    <button v-for="p in items" :key="p.key" type="button" class="tab" :aria-pressed="model === p.key" @click="model = p.key">
      <ProviderGlyph :provider="p.key" />
      <span>{{ providerLabel(p.key) }}</span>
      <span class="tab__count">{{ p.count }}</span>
    </button>
  </div>
</template>

<style scoped>
.tab__all {
  width: 17px;
  height: 17px;
}
</style>
